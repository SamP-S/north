from pathlib import Path

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    board_repo_ssh_url: str = ""
    north_home: Path = Path("~/.north").expanduser()
    board_path: Path = Path("~/.north/board").expanduser()
    poll_interval_s: int = 5
    cooldown_seconds: int = 300
    north_port: int = 8001
    # MCP grant tokens, "grant:token,grant:token" (empty = no token required)
    mcp_tokens: str = ""
    # notifications: "log" (default) or "telegram"; telegram degrades to log
    # when token/chat id are missing
    notify_transport: str = "log"
    telegram_bot_token: str = ""
    telegram_chat_id: str = ""
    notify_dedupe_window_s: int = 300
    notify_rate_limit_per_min: int = 20
    # WARNING+ log forwarding dedupes per logger+template over its own window
    log_notify_dedupe_window_s: int = 3600

    model_config = {
        "env_file": ".env",
        "env_file_encoding": "utf-8",
        "extra": "ignore",
    }


settings = Settings()
