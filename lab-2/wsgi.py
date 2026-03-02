from app import create_app
from app.models import Note

app = create_app()

__all__ = ["app", "Note"]
