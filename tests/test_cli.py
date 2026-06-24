"""CLI dispatch: init, task lifecycle, board, output modes, error path."""

import json
from pathlib import Path

from pytest import CaptureFixture, MonkeyPatch

from north.cli.main import main


def test_init_creates_board(tmp_path: Path, monkeypatch: MonkeyPatch) -> None:
    monkeypatch.chdir(tmp_path)
    assert main(["init"]) == 0
    assert (tmp_path / "north" / "config.yml").is_file()


def test_task_lifecycle(repo: Path, monkeypatch: MonkeyPatch) -> None:
    monkeypatch.chdir(repo)
    assert main(["task", "create", "Hello", "--agent", "opus4.8"]) == 0
    assert main(["task", "move", "task-1", "ready"]) == 0
    assert list((repo / "north" / "ready").glob("task-1 - *.md"))


def test_list_json_output(
    repo: Path, monkeypatch: MonkeyPatch, capsys: CaptureFixture[str]
) -> None:
    monkeypatch.chdir(repo)
    main(["task", "create", "Hello", "--labels", "a", "b"])
    capsys.readouterr()
    assert main(["task", "list", "--json"]) == 0
    payload = json.loads(capsys.readouterr().out)
    assert payload[0]["id"] == "task-1"
    assert payload[0]["labels"] == ["a", "b"]
    assert "body" not in payload[0]


def test_illegal_move_exits_nonzero(
    repo: Path, monkeypatch: MonkeyPatch, capsys: CaptureFixture[str]
) -> None:
    monkeypatch.chdir(repo)
    main(["task", "create", "Hello"])
    assert main(["task", "move", "task-1", "done"]) == 1
    assert "illegal transition" in capsys.readouterr().err


def test_board_summary(repo: Path, monkeypatch: MonkeyPatch, capsys: CaptureFixture[str]) -> None:
    monkeypatch.chdir(repo)
    main(["task", "create", "Hello"])
    capsys.readouterr()
    assert main(["board"]) == 0
    out = capsys.readouterr().out
    assert "draft" in out and "total" in out


def test_no_board_errors_cleanly(
    tmp_path: Path, monkeypatch: MonkeyPatch, capsys: CaptureFixture[str]
) -> None:
    monkeypatch.chdir(tmp_path)
    assert main(["task", "list"]) == 1
    assert "north init" in capsys.readouterr().err


def test_instructions_prints(
    repo: Path, monkeypatch: MonkeyPatch, capsys: CaptureFixture[str]
) -> None:
    monkeypatch.chdir(repo)
    assert main(["instructions"]) == 0
    assert "North" in capsys.readouterr().out
