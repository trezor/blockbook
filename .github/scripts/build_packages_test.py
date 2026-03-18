import contextlib
import io
import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import build_packages


def write_json(path: Path, payload: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload), encoding="utf-8")


class BuildPackagesTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.workspace = Path(self.tempdir.name)
        self.package_root = self.workspace / "packages"
        self.build_dir = self.workspace / "build"
        self.build_dir.mkdir(parents=True, exist_ok=True)

        write_json(
            self.workspace / "configs" / "coins" / "base_archive.json",
            {
                "coin": {"alias": "base_archive"},
                "blockbook": {"package_name": "blockbook-base"},
                "backend": {"package_name": "backend-base"},
            },
        )

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def run_build(self, *, rpc_url: str, always_build_backend: bool) -> tuple[list[str], str]:
        commands: list[list[str]] = []

        def fake_run(cmd, check, **kwargs):
            commands.append(list(cmd))
            if cmd[:1] == ["make"]:
                (self.build_dir / "blockbook-base_1.0_amd64.deb").write_text("blockbook", encoding="utf-8")
                if cmd == ["make", "deb-base_archive"]:
                    (self.build_dir / "backend-base_1.0_amd64.deb").write_text("backend", encoding="utf-8")
                return None
            raise AssertionError(f"unexpected subprocess call: {cmd}")

        env = {
            "BRANCH_OR_TAG": "feature/test-branch",
            "BB_PACKAGE_ROOT": str(self.package_root),
            "BB_BACKEND_DOMAIN": "backend.example.test",
            "BB_RPC_URL_HTTP_base_archive": rpc_url,
        }
        stdout = io.StringIO()
        old_cwd = Path.cwd()
        try:
            os.chdir(self.workspace)
            with patch.dict(os.environ, env, clear=False), patch("build_packages.subprocess.run", side_effect=fake_run):
                with contextlib.redirect_stdout(stdout):
                    argv = ["base_archive"]
                    if always_build_backend:
                        argv = ["--always-build-backend", *argv]
                    build_packages.main(argv)
        finally:
            os.chdir(old_cwd)

        return commands[-1], stdout.getvalue().strip()

    def test_builds_backend_when_rpc_url_matches_backend_domain(self) -> None:
        make_cmd, output = self.run_build(
            rpc_url="http://backend.example.test:18026",
            always_build_backend=False,
        )

        self.assertEqual(make_cmd, ["make", "deb-base_archive"])
        self.assertEqual(output, "build/blockbook-base_1.0_amd64.deb")
        staged_dir = self.package_root / "feature-test-branch" / "base_archive"
        self.assertTrue((staged_dir / "blockbook-base_1.0_amd64.deb").is_file())
        self.assertTrue((staged_dir / "backend-base_1.0_amd64.deb").is_file())

    def test_skips_backend_when_rpc_url_does_not_match_backend_domain(self) -> None:
        make_cmd, output = self.run_build(
            rpc_url="https://rpc.example.invalid/",
            always_build_backend=False,
        )

        self.assertEqual(make_cmd, ["make", "deb-blockbook-base_archive"])
        self.assertEqual(output, "build/blockbook-base_1.0_amd64.deb")
        staged_dir = self.package_root / "feature-test-branch" / "base_archive"
        self.assertTrue((staged_dir / "blockbook-base_1.0_amd64.deb").is_file())
        self.assertFalse((staged_dir / "backend-base_1.0_amd64.deb").exists())

    def test_always_build_backend_overrides_domain_matching(self) -> None:
        make_cmd, output = self.run_build(
            rpc_url="https://rpc.example.invalid/",
            always_build_backend=True,
        )

        self.assertEqual(make_cmd, ["make", "deb-base_archive"])
        self.assertEqual(output, "build/blockbook-base_1.0_amd64.deb")
        staged_dir = self.package_root / "feature-test-branch" / "base_archive"
        self.assertTrue((staged_dir / "backend-base_1.0_amd64.deb").is_file())


if __name__ == "__main__":
    unittest.main()
