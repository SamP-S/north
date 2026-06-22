"""Shared interactive prompts for the North CLI."""


def confirm(prompt: str) -> bool:
    """Prompt the user for a yes/no confirmation, defaulting to no."""
    answer = input(f"{prompt} [y/N] ").strip().lower()
    return answer in ("y", "yes")
