"""Dispatch-level tests for the top-level parser and main()."""

import pytest

from north.cli.main import _build_parser, main


def test_service_unit_propagates() -> None:
    parser = _build_parser()
    args = parser.parse_args(["service", "start"])
    assert args.unit == "north"
    assert args.func.__name__ == "start"

    args = parser.parse_args(["service", "restart"])
    assert args.unit == "north"
    assert args.func.__name__ == "restart"


def test_feature_show_two_positionals() -> None:
    parser = _build_parser()
    args = parser.parse_args(["feature", "show", "proj", "feat"])
    assert (args.project, args.feature) == ("proj", "feat")
    assert args.func.__name__ == "show"


def test_status_has_no_subcommands() -> None:
    parser = _build_parser()
    assert parser.parse_args(["status"]).func.__name__ == "status"


def test_no_command_prints_help_returns_1(capsys: pytest.CaptureFixture[str]) -> None:
    rc = main([])
    assert rc == 1
    assert "usage: north" in capsys.readouterr().out


def test_group_without_subcommand_returns_1(
    capsys: pytest.CaptureFixture[str],
) -> None:
    rc = main(["feature"])
    assert rc == 1
    assert "subcommand" in capsys.readouterr().out
