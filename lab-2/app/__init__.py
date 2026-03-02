import os

from flask import Flask

from app.config import Config
from app.extensions import db, migrate


def create_app() -> Flask:
    static_folder = os.getenv("STATIC_FOLDER", "static")
    app = Flask(__name__, static_folder=static_folder)
    app.config.from_object(Config())

    db.init_app(app)
    migrate.init_app(app, db)

    from app.routes import notes_bp

    app.register_blueprint(notes_bp)
    return app
