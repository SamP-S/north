"""Shared CLI error type.

A single :class:`CLIError` gives ``main.py`` one place to render user-facing
failures to stderr. Core :class:`~north.core.errors.BoardError` is rendered the
same way.
"""


class CLIError(Exception):
    """A user-facing CLI failure; its message is printed to stderr."""
