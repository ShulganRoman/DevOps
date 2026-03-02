import os


class Config:
    SECRET_KEY = os.getenv("SECRET_KEY", "change-me")
    DB_HOST = os.getenv("DB_HOST", "db")
    DB_PORT = int(os.getenv("DB_PORT", "5432"))
    DB_NAME = os.getenv("DB_NAME", "notes")
    DB_USER = os.getenv("DB_USER", "notes")
    DB_PASSWORD = os.getenv("DB_PASSWORD", "notes")
    SQLALCHEMY_DATABASE_URI = os.getenv(
        "DATABASE_URL",
        f"postgresql+psycopg://{DB_USER}:{DB_PASSWORD}@{DB_HOST}:{DB_PORT}/{DB_NAME}",
    )
    SQLALCHEMY_TRACK_MODIFICATIONS = False
    NOTES_PER_PAGE = int(os.getenv("NOTES_PER_PAGE", "20"))
