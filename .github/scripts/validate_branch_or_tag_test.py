import subprocess
import unittest
from unittest.mock import patch

from validate_branch_or_tag import ref_exists, validate_branch_or_tag


class ValidateBranchOrTagTest(unittest.TestCase):
    def test_accepts_existing_branch(self) -> None:
        with patch("validate_branch_or_tag.ref_exists", side_effect=lambda repo, ref, kind: kind == "heads"):
            kind = validate_branch_or_tag("trezor/blockbook", "master")
        self.assertEqual(kind, "branch")

    def test_accepts_existing_tag(self) -> None:
        with patch("validate_branch_or_tag.ref_exists", side_effect=lambda repo, ref, kind: kind == "tags"):
            kind = validate_branch_or_tag("trezor/blockbook", "v1.0.0")
        self.assertEqual(kind, "tag")

    def test_rejects_missing_ref(self) -> None:
        with patch("validate_branch_or_tag.ref_exists", return_value=False):
            with self.assertRaisesRegex(SystemExit, "1"):
                validate_branch_or_tag("trezor/blockbook", "missing-ref")


class RefExistsTest(unittest.TestCase):
    @staticmethod
    def _ls_remote(stdout: str) -> subprocess.CompletedProcess:
        return subprocess.CompletedProcess(args=[], returncode=0, stdout=stdout, stderr="")

    def test_accepts_exact_ref_name(self) -> None:
        with patch(
            "validate_branch_or_tag.subprocess.run",
            return_value=self._ls_remote("abc123\trefs/heads/master\n"),
        ):
            self.assertTrue(ref_exists("trezor/blockbook", "master", "heads"))

    def test_rejects_glob_pattern_matching_another_ref(self) -> None:
        with patch(
            "validate_branch_or_tag.subprocess.run",
            return_value=self._ls_remote("abc123\trefs/heads/master\n"),
        ):
            self.assertFalse(ref_exists("trezor/blockbook", "mas*", "heads"))

    def test_rejects_tail_match_of_longer_ref(self) -> None:
        # ls-remote patterns tail-match, so 'master' would match
        # 'feature/master' even when no branch named 'master' exists.
        with patch(
            "validate_branch_or_tag.subprocess.run",
            return_value=self._ls_remote("abc123\trefs/heads/feature/master\n"),
        ):
            self.assertFalse(ref_exists("trezor/blockbook", "master", "heads"))


if __name__ == "__main__":
    unittest.main()
