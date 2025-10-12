from typing import Optional, List, Type
from pydantic import BaseModel
import asyncio
import time

from api.inference.vllm.runner import (
    IVLLMRunner,
    VLLMRunner,
)

from common.logger import create_logger
from common.manager import IManager, ManagerState

logger = create_logger(__name__)


class InferenceInitRequest(BaseModel):
    model: str
    dtype: str
    additional_args: List[str] = []


class StartupStatus(BaseModel):
    status: str
    is_starting: bool
    is_running: bool
    elapsed_seconds: Optional[float] = None
    error: Optional[str] = None


class InferenceManager(IManager):
    def __init__(
        self,
        runner_class: Type[IVLLMRunner] = VLLMRunner
    ):
        super().__init__()
        self.vllm_runner: Optional[IVLLMRunner] = None
        self.runner_class = runner_class
        self._startup_task: Optional[asyncio.Task] = None
        self._startup_start_time: Optional[float] = None
        self._startup_error: Optional[str] = None

    def init_vllm(
        self,
        init_request: InferenceInitRequest
    ):
        if self.is_running():
            raise Exception("VLLMRunner is already running. Stop it first.")
        
        self.vllm_runner = self.runner_class(
            model=init_request.model,
            dtype=init_request.dtype,
            additional_args=init_request.additional_args,
        )

    def _start(self):
        if self.vllm_runner is None:
            raise Exception("VLLMRunner not initialized")
        if self.is_running():
            raise Exception("VLLMRunner is already running")
        self.vllm_runner.start()
        logger.info("VLLMRunner started")

    def _stop(self):
        if self._startup_task and not self._startup_task.done():
            self._startup_task.cancel()
            logger.info("Cancelled ongoing startup task")
        
        if self.vllm_runner:
            self.vllm_runner.stop()
            logger.info("VLLMRunner stopped")
        self.vllm_runner = None
        self._exception = None

    def is_running(self) -> bool:
        return self.vllm_runner is not None and self.vllm_runner.is_running()

    def _is_healthy(self) -> bool:
        if self.vllm_runner is None:
            return True

        is_alive = self.vllm_runner.is_alive()
        if not is_alive:
            error = self.vllm_runner.get_error_if_exist()
            if error:
                logger.error(f"VLLMRunner is not alive: {error}")

        return is_alive

    def start_async(self, init_request: InferenceInitRequest):
        if self._startup_task and not self._startup_task.done():
            raise ValueError("Startup is already in progress")
        
        if self.is_running():
            raise ValueError("VLLM is already running")
        
        self._startup_start_time = time.time()
        self._startup_error = None
        self._startup_task = asyncio.create_task(
            self._async_startup_worker(init_request)
        )
    
    async def _async_startup_worker(self, init_request: InferenceInitRequest):
        try:
            loop = asyncio.get_event_loop()
            await loop.run_in_executor(None, self._do_startup, init_request)
            logger.info("Async startup completed successfully")
        except asyncio.CancelledError:
            logger.info("Async startup was cancelled")
            self._startup_error = "Startup was cancelled"
            raise
        except Exception as e:
            logger.error(f"Async startup failed: {e}")
            self._startup_error = str(e)
            raise
    
    def _do_startup(self, init_request: InferenceInitRequest):
        try:
            self.init_vllm(init_request)
            self._state = ManagerState.STARTING
            self._start()
            self._is_active = True
            self._state = ManagerState.RUNNING
        except Exception as e:
            if self.vllm_runner is None:
                logger.info(f"Startup stopped gracefully: {e}")
                self._state = ManagerState.STOPPED
                return
            
            logger.error(f"Failed to start {self.__class__.__name__}: {e}")
            self._exception = e
            self._state = ManagerState.FAILED
            raise
    
    def is_starting(self) -> bool:
        return self._startup_task is not None and not self._startup_task.done()
    
    def get_startup_status(self) -> StartupStatus:
        if not self._startup_task:
            return StartupStatus(
                status="not_started",
                is_starting=False,
                is_running=self.is_running()
            )
        
        if self._startup_task.done():
            try:
                self._startup_task.result()
                return StartupStatus(
                    status="completed",
                    is_starting=False,
                    is_running=self.is_running(),
                    elapsed_seconds=time.time() - self._startup_start_time if self._startup_start_time else 0
                )
            except asyncio.CancelledError:
                return StartupStatus(
                    status="cancelled",
                    is_starting=False,
                    is_running=self.is_running(),
                    error=self._startup_error
                )
            except Exception as e:
                return StartupStatus(
                    status="failed",
                    is_starting=False,
                    is_running=self.is_running(),
                    error=str(e)
                )
        
        elapsed = time.time() - self._startup_start_time if self._startup_start_time else 0
        return StartupStatus(
            status="in_progress",
            is_starting=True,
            is_running=False,
            elapsed_seconds=elapsed
        )
