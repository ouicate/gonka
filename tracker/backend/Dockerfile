FROM python:3.11-slim

WORKDIR /app

RUN pip install --no-cache-dir uv

COPY pyproject.toml .
RUN uv pip install --system --no-cache-dir -e .

COPY src ./src

CMD ["uvicorn", "src.backend.app:app", "--host", "0.0.0.0", "--port", "8000"]

