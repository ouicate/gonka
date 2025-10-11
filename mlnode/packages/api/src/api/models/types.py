"""Type definitions for model management."""

from pydantic import BaseModel, Field
from typing import Optional, List
from enum import Enum


class Model(BaseModel):
    """Represents a HuggingFace model identifier.
    
    Attributes:
        hf_repo: HuggingFace repository ID (e.g., "meta-llama/Llama-2-7b-hf")
        hf_commit: Optional commit hash. If None, uses the latest version.
    """
    hf_repo: str = Field(..., description="HuggingFace repository ID")
    hf_commit: Optional[str] = Field(None, description="Specific commit hash (optional)")

    def get_identifier(self) -> str:
        """Generate a unique identifier for this model."""
        if self.hf_commit:
            return f"{self.hf_repo}:{self.hf_commit}"
        return f"{self.hf_repo}:latest"


class ModelStatus(str, Enum):
    """Status of a model in the cache."""
    DOWNLOADED = "DOWNLOADED"  # Model fully downloaded and verified
    DOWNLOADING = "DOWNLOADING"  # Download in progress
    NOT_FOUND = "NOT_FOUND"  # Model not in cache
    ERROR = "ERROR"  # Download or verification failed
    PARTIAL = "PARTIAL"  # Partial download (e.g., cancelled)


class DownloadProgress(BaseModel):
    """Progress information for model download.
    
    Attributes:
        start_time: Unix timestamp when download started
        elapsed_seconds: Seconds elapsed since download started
    """
    start_time: float = Field(..., description="Download start time (Unix timestamp)")
    elapsed_seconds: float = Field(..., description="Seconds elapsed since start")


class ModelStatusResponse(BaseModel):
    """Response containing model status information.
    
    Attributes:
        model: The model being queried
        status: Current status of the model
        progress: Download progress (only present when status is DOWNLOADING)
        error_message: Error description (only present when status is ERROR)
    """
    model: Model
    status: ModelStatus
    progress: Optional[DownloadProgress] = None
    error_message: Optional[str] = None


class DownloadStartResponse(BaseModel):
    """Response when starting a model download.
    
    Attributes:
        task_id: Unique identifier for the download task
        status: Initial status (should be DOWNLOADING)
        model: The model being downloaded
    """
    task_id: str = Field(..., description="Unique task identifier")
    status: ModelStatus = Field(..., description="Download status")
    model: Model


class DeleteResponse(BaseModel):
    """Response when deleting a model.
    
    Attributes:
        status: Action taken (deleted or cancelled)
        model: The model that was deleted/cancelled
    """
    status: str = Field(..., description="Action taken: 'deleted' or 'cancelled'")
    model: Model


class ModelListResponse(BaseModel):
    """Response containing list of cached models.
    
    Attributes:
        models: List of models in the cache
    """
    models: List[Model] = Field(..., description="List of cached models")


class DiskSpaceInfo(BaseModel):
    """Information about disk space usage.
    
    Attributes:
        cache_size_bytes: Total size of HuggingFace cache in bytes
        available_bytes: Available disk space in bytes
        cache_path: Path to the HuggingFace cache directory
    """
    cache_size_bytes: int = Field(..., description="Total cache size in bytes")
    available_bytes: int = Field(..., description="Available disk space in bytes")
    cache_path: str = Field(..., description="Path to cache directory")

