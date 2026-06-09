import json
import tempfile
import unittest
from pathlib import Path

import validate_backend_artifacts


class BackendArtifactLintTest(unittest.TestCase):
    def lint_config(self, backend: dict) -> tuple[list[str], list[str]]:
        with tempfile.TemporaryDirectory() as tempdir:
            path = Path(tempdir) / "coin.json"
            path.write_text(json.dumps({"backend": backend}), encoding="utf-8")
            return validate_backend_artifacts.lint_file(path)

    def test_lints_platform_backend_after_merging_top_level_defaults(self) -> None:
        errors, warnings = self.lint_config(
            {
                "binary_url": (
                    "https://example.com/releases/download/v1.0.0/backend-linux-amd64.tar.gz"
                ),
                "verification_type": "sha256",
                "verification_source": "abc123",
                "extract_command": "tar -C backend --strip 1 -xf",
                "platforms": {
                    "arm64": {
                        "binary_url": (
                            "https://raw.githubusercontent.com/example/backend/main/"
                            "backend-linux-arm64.tar.gz"
                        )
                    }
                },
            }
        )

        self.assertEqual(warnings, [])
        self.assertEqual(
            errors,
            [
                "backend.platforms.arm64: binary_url uses a mutable branch ref: "
                "https://raw.githubusercontent.com/example/backend/main/"
                "backend-linux-arm64.tar.gz"
            ],
        )


if __name__ == "__main__":
    unittest.main()
