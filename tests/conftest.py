"""Shared fixtures: a scaffolded in-repo board in a tmp directory."""

from pathlib import Path

import pytest

from north.core import board as board_mod


@pytest.fixture
def repo(tmp_path: Path) -> Path:
    """A project directory with a scaffolded `north/` board."""
    board_mod.init_board(tmp_path)
    return tmp_path


@pytest.fixture
def board(repo: Path) -> Path:
    """The `north/` board directory inside :func:`repo`."""
    return repo / "north"
