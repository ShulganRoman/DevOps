from flask import Blueprint, current_app, flash, redirect, render_template, request, url_for

from app.extensions import db
from app.models import Note

notes_bp = Blueprint("notes", __name__)


@notes_bp.route("/", methods=["GET"])
def index():
    notes = (
        Note.query.order_by(Note.updated_at.desc(), Note.id.desc())
        .limit(current_app.config["NOTES_PER_PAGE"])
        .all()
    )
    return render_template("index.html", notes=notes)


@notes_bp.route("/notes", methods=["POST"])
def create_note():
    title = request.form.get("title", "").strip()
    content = request.form.get("content", "").strip()

    if not title or not content:
        flash("Title and content are required.")
        return redirect(url_for("notes.index"))

    db.session.add(Note(title=title, content=content))
    db.session.commit()
    flash("Note created.")
    return redirect(url_for("notes.index"))


@notes_bp.route("/notes/<int:note_id>/edit", methods=["GET", "POST"])
def edit_note(note_id: int):
    note = Note.query.get_or_404(note_id)

    if request.method == "POST":
        title = request.form.get("title", "").strip()
        content = request.form.get("content", "").strip()

        if not title or not content:
            flash("Title and content are required.")
            return render_template("edit.html", note=note)

        note.title = title
        note.content = content
        db.session.commit()
        flash("Note updated.")
        return redirect(url_for("notes.index"))

    return render_template("edit.html", note=note)


@notes_bp.route("/notes/<int:note_id>/delete", methods=["POST"])
def delete_note(note_id: int):
    note = Note.query.get_or_404(note_id)
    db.session.delete(note)
    db.session.commit()
    flash("Note deleted.")
    return redirect(url_for("notes.index"))
