FROM python:3.12-alpine

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    PATH="/opt/venv/bin:${PATH}"

WORKDIR /app

RUN apk add --no-cache postgresql-client \
    && python -m venv /opt/venv

COPY requirements.txt .

RUN pip install --upgrade pip \
    && pip install -r requirements.txt

RUN addgroup -S notesapp && adduser -S -G notesapp notesapp

COPY --chown=notesapp:notesapp app /app/app
COPY --chown=notesapp:notesapp migrations /app/migrations
COPY --chown=notesapp:notesapp wsgi.py /app/wsgi.py
COPY --chown=notesapp:notesapp entrypoint.sh /app/entrypoint.sh
COPY --chown=notesapp:notesapp app/static_seed /app/static-data

RUN chmod +x /app/entrypoint.sh \
    && chown -R notesapp:notesapp /app /opt/venv

USER notesapp

ENV FLASK_APP=wsgi.py \
    PORT=8080 \
    STATIC_FOLDER=/app/static-data \
    STATIC_SOURCE_DIR=/app/app/static_seed \
    STATIC_TARGET_DIR=/app/static-data

EXPOSE 8080

ENTRYPOINT ["/app/entrypoint.sh"]
