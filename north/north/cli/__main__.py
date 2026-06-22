"""Allow ``python -m north.cli`` to run the CLI."""

from north.cli.main import _entrypoint

if __name__ == "__main__":
    _entrypoint()
