import subprocess
import sys
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("run.py")


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


if __name__ == "__main__":
    unittest.main()
