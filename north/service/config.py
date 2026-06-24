"""Service settings for the optional MCP server.

The board itself is discovered on disk and configured via ``north/config.yml``
(e.g. ``mcp_port``). The only thing here is the optional bearer token, which is
a secret and so lives in the environment, never in the committed board config.
"""

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    # Optional bearer token required on MCP requests (empty = no auth).
    mcp_token: str = ""

    model_config = {
        "env_file": ".env",
        "env_file_encoding": "utf-8",
        "extra": "ignore",
    }


settings = Settings()
