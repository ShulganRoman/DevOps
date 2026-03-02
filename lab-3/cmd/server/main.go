package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultAppName         = "Notes Service"
	defaultHost            = "0.0.0.0"
	defaultPort            = 8080
	defaultUsers           = "demo:demo"
	defaultSessionCookie   = "notes_session"
	defaultReadTimeout     = 5 * time.Second
	defaultWriteTimeout    = 10 * time.Second
	defaultShutdownTimeout = 10 * time.Second
	maxTitleLength         = 120
	maxContentLength       = 5000
)

type config struct {
	AppName         string
	Host            string
	Port            int
	SessionCookie   string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

type app struct {
	cfg       config
	users     map[string]string
	notes     *noteStore
	sessions  *sessionStore
	templates *template.Template
}

type Note struct {
	ID        int64
	Title     string
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type noteStore struct {
	mu     sync.RWMutex
	nextID int64
	byUser map[string][]Note
}

type sessionStore struct {
	mu      sync.RWMutex
	byToken map[string]string
}

type loginPageData struct {
	AppName  string
	Error    string
	Username string
	Users    []string
}

type notesPageData struct {
	AppName  string
	Username string
	Notes    []Note
}

type noteFormPageData struct {
	AppName     string
	Username    string
	PageTitle   string
	SubmitLabel string
	Action      string
	Error       string
	Note        Note
}

func main() {
	cfg := loadConfig()
	users := loadUsers()

	app := &app{
		cfg:       cfg,
		users:     users,
		notes:     newNoteStore(),
		sessions:  newSessionStore(),
		templates: newTemplates(),
	}

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      app.routes(),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	log.Printf("starting %s on %s", cfg.AppName, server.Addr)

	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
		<-signals

		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server exited with error: %v", err)
	}

	log.Println("server stopped")
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleHome)
	mux.HandleFunc("/healthz", a.handleHealth)
	mux.HandleFunc("/login", a.handleLogin)
	mux.HandleFunc("/logout", a.handleLogout)
	mux.HandleFunc("/notes/new", a.handleNewNote)
	mux.HandleFunc("/notes/create", a.handleCreateNote)
	mux.HandleFunc("/notes/", a.handleNoteAction)

	return mux
}

func (a *app) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	data := notesPageData{
		AppName:  a.cfg.AppName,
		Username: username,
		Notes:    a.notes.List(username),
	}

	a.renderTemplate(w, "notes", data)
}

func (a *app) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (a *app) handleLogin(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.currentUser(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.renderTemplate(w, "login", loginPageData{
			AppName: a.cfg.AppName,
			Users:   sortedUsernames(a.users),
		})
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}

		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")

		expectedPassword, ok := a.users[username]
		if !ok || expectedPassword != password {
			a.renderTemplate(w, "login", loginPageData{
				AppName:  a.cfg.AppName,
				Error:    "Неверный логин или пароль",
				Username: username,
				Users:    sortedUsernames(a.users),
			})
			return
		}

		token, err := a.sessions.Create(username)
		if err != nil {
			http.Error(w, "failed to create session", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     a.cfg.SessionCookie,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(w, r, "/", http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *app) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cookie, err := r.Cookie(a.cfg.SessionCookie); err == nil && cookie.Value != "" {
		a.sessions.Delete(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     a.cfg.SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (a *app) handleNewNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	a.renderTemplate(w, "note_form", noteFormPageData{
		AppName:     a.cfg.AppName,
		Username:    username,
		PageTitle:   "Новая заметка",
		SubmitLabel: "Сохранить",
		Action:      "/notes/create",
	})
}

func (a *app) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	note, errMessage := parseNoteForm(r)
	if errMessage != "" {
		a.renderTemplate(w, "note_form", noteFormPageData{
			AppName:     a.cfg.AppName,
			Username:    username,
			PageTitle:   "Новая заметка",
			SubmitLabel: "Сохранить",
			Action:      "/notes/create",
			Error:       errMessage,
			Note:        note,
		})
		return
	}

	a.notes.Create(username, note.Title, note.Content)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *app) handleNoteAction(w http.ResponseWriter, r *http.Request) {
	username, ok := a.requireAuth(w, r)
	if !ok {
		return
	}

	trimmed := strings.TrimPrefix(r.URL.Path, "/notes/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}

	noteID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	switch parts[1] {
	case "edit":
		a.handleEditNote(w, r, username, noteID)
	case "update":
		a.handleUpdateNote(w, r, username, noteID)
	case "delete":
		a.handleDeleteNote(w, r, username, noteID)
	default:
		http.NotFound(w, r)
	}
}

func (a *app) handleEditNote(w http.ResponseWriter, r *http.Request, username string, noteID int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	note, ok := a.notes.Get(username, noteID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	a.renderTemplate(w, "note_form", noteFormPageData{
		AppName:     a.cfg.AppName,
		Username:    username,
		PageTitle:   "Редактирование заметки",
		SubmitLabel: "Обновить",
		Action:      fmt.Sprintf("/notes/%d/update", noteID),
		Note:        note,
	})
}

func (a *app) handleUpdateNote(w http.ResponseWriter, r *http.Request, username string, noteID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	existingNote, ok := a.notes.Get(username, noteID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	note, errMessage := parseNoteForm(r)
	if errMessage != "" {
		note.ID = existingNote.ID
		note.CreatedAt = existingNote.CreatedAt
		note.UpdatedAt = existingNote.UpdatedAt
		a.renderTemplate(w, "note_form", noteFormPageData{
			AppName:     a.cfg.AppName,
			Username:    username,
			PageTitle:   "Редактирование заметки",
			SubmitLabel: "Обновить",
			Action:      fmt.Sprintf("/notes/%d/update", noteID),
			Error:       errMessage,
			Note:        note,
		})
		return
	}

	if !a.notes.Update(username, noteID, note.Title, note.Content) {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *app) handleDeleteNote(w http.ResponseWriter, r *http.Request, username string, noteID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !a.notes.Delete(username, noteID) {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *app) renderTemplate(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := a.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template render failed: %v", err)
		http.Error(w, "template render failed", http.StatusInternalServerError)
	}
}

func (a *app) requireAuth(w http.ResponseWriter, r *http.Request) (string, bool) {
	username, ok := a.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return "", false
	}

	return username, true
}

func (a *app) currentUser(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(a.cfg.SessionCookie)
	if err != nil || cookie.Value == "" {
		return "", false
	}

	username, ok := a.sessions.Get(cookie.Value)
	return username, ok
}

func newNoteStore() *noteStore {
	return &noteStore{
		byUser: make(map[string][]Note),
	}
}

func (s *noteStore) List(username string) []Note {
	s.mu.RLock()
	defer s.mu.RUnlock()

	notes := append([]Note(nil), s.byUser[username]...)
	sort.Slice(notes, func(i, j int) bool {
		return notes[i].UpdatedAt.After(notes[j].UpdatedAt)
	})

	return notes
}

func (s *noteStore) Get(username string, noteID int64) (Note, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, note := range s.byUser[username] {
		if note.ID == noteID {
			return note, true
		}
	}

	return Note{}, false
}

func (s *noteStore) Create(username, title, content string) Note {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	now := time.Now().UTC()
	note := Note{
		ID:        s.nextID,
		Title:     title,
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.byUser[username] = append(s.byUser[username], note)
	return note
}

func (s *noteStore) Update(username string, noteID int64, title, content string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	notes := s.byUser[username]
	for idx := range notes {
		if notes[idx].ID != noteID {
			continue
		}

		notes[idx].Title = title
		notes[idx].Content = content
		notes[idx].UpdatedAt = time.Now().UTC()
		s.byUser[username] = notes
		return true
	}

	return false
}

func (s *noteStore) Delete(username string, noteID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	notes := s.byUser[username]
	for idx := range notes {
		if notes[idx].ID != noteID {
			continue
		}

		s.byUser[username] = append(notes[:idx], notes[idx+1:]...)
		return true
	}

	return false
}

func newSessionStore() *sessionStore {
	return &sessionStore{
		byToken: make(map[string]string),
	}
}

func (s *sessionStore) Create(username string) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}

	token := hex.EncodeToString(tokenBytes)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.byToken[token] = username

	return token, nil
}

func (s *sessionStore) Get(token string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	username, ok := s.byToken[token]
	return username, ok
}

func (s *sessionStore) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byToken, token)
}

func parseNoteForm(r *http.Request) (Note, string) {
	if err := r.ParseForm(); err != nil {
		return Note{}, "Не удалось прочитать форму"
	}

	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))

	switch {
	case title == "":
		return Note{Title: title, Content: content}, "Заголовок обязателен"
	case len([]rune(title)) > maxTitleLength:
		return Note{Title: title, Content: content}, fmt.Sprintf("Заголовок не должен превышать %d символов", maxTitleLength)
	case content == "":
		return Note{Title: title, Content: content}, "Текст заметки обязателен"
	case len([]rune(content)) > maxContentLength:
		return Note{Title: title, Content: content}, fmt.Sprintf("Текст заметки не должен превышать %d символов", maxContentLength)
	default:
		return Note{Title: title, Content: content}, ""
	}
}

func loadConfig() config {
	return config{
		AppName:         getEnv("APP_NAME", defaultAppName),
		Host:            getEnv("APP_HOST", defaultHost),
		Port:            getEnvAsInt("APP_PORT", defaultPort),
		SessionCookie:   getEnv("APP_SESSION_COOKIE", defaultSessionCookie),
		ReadTimeout:     getEnvAsDuration("APP_READ_TIMEOUT", defaultReadTimeout),
		WriteTimeout:    getEnvAsDuration("APP_WRITE_TIMEOUT", defaultWriteTimeout),
		ShutdownTimeout: getEnvAsDuration("APP_SHUTDOWN_TIMEOUT", defaultShutdownTimeout),
	}
}

func loadUsers() map[string]string {
	rawUsers := getEnv("APP_USERS", defaultUsers)
	users := make(map[string]string)

	for _, pair := range strings.Split(rawUsers, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		username, password, ok := strings.Cut(pair, ":")
		username = strings.TrimSpace(username)
		password = strings.TrimSpace(password)
		if !ok || username == "" || password == "" {
			log.Printf("invalid user entry %q, skipping", pair)
			continue
		}

		users[username] = password
	}

	if len(users) == 0 {
		users["demo"] = "demo"
	}

	return users
}

func newTemplates() *template.Template {
	funcs := template.FuncMap{
		"formatTime": func(value time.Time) string {
			if value.IsZero() {
				return "-"
			}

			return value.Local().Format("02.01.2006 15:04")
		},
	}

	return template.Must(template.New("pages").Funcs(funcs).Parse(pageTemplates))
}

func sortedUsernames(users map[string]string) []string {
	names := make([]string, 0, len(users))
	for username := range users {
		names = append(names, username)
	}

	sort.Strings(names)
	return names
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return value
	}

	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	value := getEnv(key, "")
	if value == "" {
		return fallback
	}

	number, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("invalid integer for %s=%q, using default %d", key, value, fallback)
		return fallback
	}

	return number
}

func getEnvAsDuration(key string, fallback time.Duration) time.Duration {
	value := getEnv(key, "")
	if value == "" {
		return fallback
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		log.Printf("invalid duration for %s=%q, using default %s", key, value, fallback)
		return fallback
	}

	return duration
}

const pageTemplates = `
{{define "login"}}
<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.AppName}} | Вход</title>
  <style>
    :root {
      color-scheme: light;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: #f4f6fb;
      color: #172033;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      display: grid;
      place-items: center;
      background: linear-gradient(135deg, #eef4ff, #f9fbff 40%, #eef8f3);
    }
    .card {
      width: min(100%, 460px);
      background: #ffffff;
      border: 1px solid #dbe4f0;
      border-radius: 20px;
      padding: 32px;
      box-shadow: 0 20px 60px rgba(19, 38, 77, 0.08);
    }
    h1 { margin: 0 0 12px; font-size: 28px; }
    p { margin: 0 0 20px; color: #53627c; }
    label { display: block; margin: 16px 0 8px; font-weight: 600; }
    input {
      width: 100%;
      padding: 12px 14px;
      border: 1px solid #c9d5e6;
      border-radius: 12px;
      font: inherit;
    }
    button {
      width: 100%;
      margin-top: 20px;
      padding: 12px 14px;
      border: 0;
      border-radius: 12px;
      background: #1f6feb;
      color: #fff;
      font: inherit;
      font-weight: 700;
      cursor: pointer;
    }
    .error {
      margin-bottom: 12px;
      padding: 10px 12px;
      border-radius: 12px;
      background: #fff2f2;
      color: #a93c3c;
      border: 1px solid #f3caca;
    }
    .hint {
      margin-top: 18px;
      padding-top: 18px;
      border-top: 1px solid #e7edf5;
      font-size: 14px;
      color: #53627c;
    }
    code {
      padding: 2px 6px;
      border-radius: 6px;
      background: #eef4ff;
      font-size: 13px;
    }
  </style>
</head>
<body>
  <main class="card">
    <h1>{{.AppName}}</h1>
    <p>Войдите, чтобы просматривать и редактировать свои заметки.</p>
    {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
    <form method="post" action="/login">
      <label for="username">Пользователь</label>
      <input id="username" name="username" autocomplete="username" value="{{.Username}}" required>
      <label for="password">Пароль</label>
      <input id="password" type="password" name="password" autocomplete="current-password" required>
      <button type="submit">Войти</button>
    </form>
    <div class="hint">
      Пользователи задаются через <code>APP_USERS</code> в формате <code>user:password,user2:password2</code>.
      {{if .Users}}Сейчас доступны: {{range $idx, $user := .Users}}{{if $idx}}, {{end}}<code>{{$user}}</code>{{end}}.{{end}}
    </div>
  </main>
</body>
</html>
{{end}}

{{define "notes"}}
<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.AppName}} | Мои заметки</title>
  <style>
    :root {
      color-scheme: light;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: #f4f6fb;
      color: #172033;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background: linear-gradient(160deg, #f7faff, #eef4ff 35%, #f8fbff);
    }
    .shell {
      max-width: 960px;
      margin: 0 auto;
      padding: 32px 20px 48px;
    }
    .topbar {
      display: flex;
      flex-wrap: wrap;
      gap: 12px;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 24px;
    }
    .brand h1 {
      margin: 0;
      font-size: 30px;
    }
    .brand p {
      margin: 8px 0 0;
      color: #53627c;
    }
    .actions {
      display: flex;
      gap: 10px;
      align-items: center;
    }
    .button, button {
      padding: 10px 14px;
      border: 0;
      border-radius: 12px;
      font: inherit;
      font-weight: 700;
      cursor: pointer;
      text-decoration: none;
    }
    .button {
      background: #1f6feb;
      color: #fff;
    }
    .ghost {
      background: #eaf1fb;
      color: #17315f;
    }
    .grid {
      display: grid;
      gap: 16px;
    }
    .empty, .note {
      background: #fff;
      border: 1px solid #dbe4f0;
      border-radius: 18px;
      padding: 20px;
      box-shadow: 0 16px 40px rgba(19, 38, 77, 0.06);
    }
    .note header {
      display: flex;
      gap: 12px;
      align-items: flex-start;
      justify-content: space-between;
      margin-bottom: 10px;
    }
    .note h2 {
      margin: 0;
      font-size: 22px;
    }
    .meta {
      margin: 0;
      color: #6a7891;
      font-size: 14px;
    }
    .content {
      margin: 0 0 16px;
      white-space: pre-wrap;
      line-height: 1.55;
    }
    .inline-actions {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
    }
    form { margin: 0; }
    @media (max-width: 640px) {
      .topbar, .note header { align-items: stretch; }
      .actions, .inline-actions { width: 100%; }
      .button, button { flex: 1; text-align: center; }
    }
  </style>
</head>
<body>
  <div class="shell">
    <div class="topbar">
      <div class="brand">
        <h1>{{.AppName}}</h1>
        <p>Пользователь: <strong>{{.Username}}</strong></p>
      </div>
      <div class="actions">
        <a class="button" href="/notes/new">Новая заметка</a>
        <form method="post" action="/logout">
          <button class="ghost" type="submit">Выйти</button>
        </form>
      </div>
    </div>

    {{if .Notes}}
      <div class="grid">
        {{range .Notes}}
          <article class="note">
            <header>
              <div>
                <h2>{{.Title}}</h2>
                <p class="meta">Обновлено: {{formatTime .UpdatedAt}}</p>
              </div>
            </header>
            <p class="content">{{.Content}}</p>
            <div class="inline-actions">
              <a class="button" href="/notes/{{.ID}}/edit">Редактировать</a>
              <form method="post" action="/notes/{{.ID}}/delete">
                <button class="ghost" type="submit">Удалить</button>
              </form>
            </div>
          </article>
        {{end}}
      </div>
    {{else}}
      <div class="empty">
        У вас пока нет заметок. Создайте первую.
      </div>
    {{end}}
  </div>
</body>
</html>
{{end}}

{{define "note_form"}}
<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.AppName}} | {{.PageTitle}}</title>
  <style>
    :root {
      color-scheme: light;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: #f4f6fb;
      color: #172033;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      background: linear-gradient(160deg, #f7faff, #eef4ff 35%, #f8fbff);
    }
    .shell {
      max-width: 860px;
      margin: 0 auto;
      padding: 32px 20px 48px;
    }
    .card {
      background: #fff;
      border: 1px solid #dbe4f0;
      border-radius: 20px;
      padding: 24px;
      box-shadow: 0 20px 50px rgba(19, 38, 77, 0.07);
    }
    .topbar {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
      margin-bottom: 18px;
    }
    h1 {
      margin: 0;
      font-size: 28px;
    }
    .link {
      text-decoration: none;
      color: #1f6feb;
      font-weight: 700;
    }
    .error {
      margin-bottom: 12px;
      padding: 10px 12px;
      border-radius: 12px;
      background: #fff2f2;
      color: #a93c3c;
      border: 1px solid #f3caca;
    }
    label {
      display: block;
      margin: 16px 0 8px;
      font-weight: 600;
    }
    input, textarea {
      width: 100%;
      padding: 12px 14px;
      border: 1px solid #c9d5e6;
      border-radius: 12px;
      font: inherit;
      resize: vertical;
    }
    textarea { min-height: 220px; }
    button {
      margin-top: 18px;
      padding: 12px 16px;
      border: 0;
      border-radius: 12px;
      background: #1f6feb;
      color: #fff;
      font: inherit;
      font-weight: 700;
      cursor: pointer;
    }
  </style>
</head>
<body>
  <div class="shell">
    <div class="topbar">
      <h1>{{.PageTitle}}</h1>
      <a class="link" href="/">Назад к списку</a>
    </div>
    <div class="card">
      {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
      <form method="post" action="{{.Action}}">
        <label for="title">Заголовок</label>
        <input id="title" name="title" maxlength="120" value="{{.Note.Title}}" required>
        <label for="content">Текст заметки</label>
        <textarea id="content" name="content" maxlength="5000" required>{{.Note.Content}}</textarea>
        <button type="submit">{{.SubmitLabel}}</button>
      </form>
    </div>
  </div>
</body>
</html>
{{end}}
`
