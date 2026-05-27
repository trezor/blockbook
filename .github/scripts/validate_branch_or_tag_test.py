import unittest
from unittest.mock import patch

from validate_branch_or_tag import validate_branch_or_tag


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


if __name__ == "__main__":
    unittest.main()
