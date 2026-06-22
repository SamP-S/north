"""Shared CLI error type.

A single :class:`CLIError` is raised by the board client so that ``main.py``'s
one ``except CLIError`` clause and the command handlers' error handling share a
single human-readable failure path.
"""


class CLIError(Exception):
    """A user-facing CLI failure; its message is printed to stderr."""
