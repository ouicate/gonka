from fastapi import APIRouter

router = APIRouter(prefix="/v1")

@router.get("/hello")
def hello():
    return {"message": "hello"}

