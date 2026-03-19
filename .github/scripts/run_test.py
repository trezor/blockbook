import importlib.util
import subprocess
import sys
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch


SCRIPT = Path(__file__).with_name("run.py")
SCRIPT_DIR = SCRIPT.parent


def load_run_module():
    sys.path.insert(0, str(SCRIPT_DIR))
    try:
        spec = importlib.util.spec_from_file_location("run_under_test", SCRIPT)
        module = importlib.util.module_from_spec(spec)
        assert spec is not None and spec.loader is not None
        spec.loader.exec_module(module)
        return module
    finally:
        sys.path.pop(0)


def run_cli(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [sys.executable, str(SCRIPT), *args],
        check=False,
        capture_output=True,
        text=True,
    )


class RunCliHelpTest(unittest.TestCase):
    def test_top_level_help_mentions_subcommand_help(self) -> None:
        result = run_cli("--help")
        self.assertEqual(result.returncode, 0)
        self.assertIn("Use '<command> --help' for command-specific options.", result.stdout)

    def test_build_help_is_subcommand_specific(self) -> None:
        result = run_cli("build", "--help")
        self.assertEqual(result.returncode, 0)
        self.assertIn("--always-build-backend", result.stdout)
        self.assertIn("--coins", result.stdout)
        self.assertNotIn("--format", result.stdout)

    def test_list_help_is_subcommand_specific(self) -> None:
        result = run_cli("list", "--help")
        self.assertEqual(result.returncode, 0)
        self.assertIn("--format", result.stdout)
        self.assertNotIn("--always-build-backend", result.stdout)

    def test_help_subcommand_can_show_build_help(self) -> None:
        result = run_cli("help", "build")
        self.assertEqual(result.returncode, 0)
        self.assertIn("--always-build-backend", result.stdout)
        self.assertIn("Build Debian packages only.", result.stdout)


class RunWatchTest(unittest.TestCase):
    def test_watch_completed_run_shows_logs(self) -> None:
        module = load_run_module()
        args = SimpleNamespace(run_id="123", repo="trezor/blockbook")

        with patch.object(module, "run_metadata", return_value={"status": "completed"}), patch.object(
            module, "show_run_logs"
        ) as show_logs, patch.object(module.subprocess, "run") as subproc_run:
            module.handle_watch(args)

        show_logs.assert_called_once_with("trezor/blockbook", "123")
        subproc_run.assert_not_called()

    def test_watch_in_progress_run_uses_gh_watch(self) -> None:
        module = load_run_module()
        args = SimpleNamespace(run_id="123", repo="trezor/blockbook")

        with patch.object(module, "run_metadata", return_value={"status": "in_progress"}), patch.object(
            module, "show_run_logs"
        ) as show_logs, patch.object(module.subprocess, "run") as subproc_run:
            module.handle_watch(args)

        show_logs.assert_not_called()
        subproc_run.assert_called_once_with(
            ["gh", "run", "watch", "-R", "trezor/blockbook", "123"], check=True
        )


if __name__ == "__main__":
    unittest.main()
