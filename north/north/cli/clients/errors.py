"""Shared CLI error type.

A single :class:`CLIError` is raised by both the Aurora and Borealis clients so
that ``main.py``'s one ``except CLIError`` clause and ``north status``'s
per-block error handling work regardless of which service raised.
"""


class CLIError(Exception):
    """A user-facing CLI failure; its message is printed to stderr."""
