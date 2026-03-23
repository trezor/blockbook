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
        write_json(
            self.workspace / "configs" / "coins" / "polygon_archive.json",
            {
                "coin": {"alias": "polygon_archive_bor"},
                "blockbook": {"package_name": "blockbook-polygon"},
                "backend": {"package_name": "backend-polygon"},
            },
        )

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def run_build(
        self,
        *,
        coin: str,
        build_env: str | None = None,
        rpc_env: str | None = None,
        rpc_url: str | None = None,
        always_build_backend: bool,
    ) -> tuple[list[str], str]:
        commands: list[list[str]] = []
        outputs = {
            "deb-base_archive": ("blockbook-base_1.0_amd64.deb", "backend-base_1.0_amd64.deb"),
            "deb-blockbook-base_archive": ("blockbook-base_1.0_amd64.deb", None),
            "deb-polygon_archive": (
                "blockbook-polygon_1.0_amd64.deb",
                "backend-polygon_1.0_amd64.deb",
            ),
            "deb-blockbook-polygon_archive": ("blockbook-polygon_1.0_amd64.deb", None),
        }

        def fake_run(cmd, check, **kwargs):
            commands.append(list(cmd))
            if cmd[:1] == ["make"]:
                target = cmd[1]
                blockbook_name, backend_name = outputs[target]
                (self.build_dir / blockbook_name).write_text("blockbook", encoding="utf-8")
                if backend_name:
                    (self.build_dir / backend_name).write_text("backend", encoding="utf-8")
                return None
            raise AssertionError(f"unexpected subprocess call: {cmd}")

        env = {
            "BRANCH_OR_TAG": "feature/test-branch",
            "BB_PACKAGE_ROOT": str(self.package_root),
        }
        if build_env is not None:
            env["BB_BUILD_ENV"] = build_env
        if rpc_env is not None and rpc_url is not None:
            env[rpc_env] = rpc_url
        stdout = io.StringIO()
        old_cwd = Path.cwd()
        try:
            os.chdir(self.workspace)
            with patch.dict(os.environ, env, clear=True), patch("build_packages.subprocess.run", side_effect=fake_run):
                with contextlib.redirect_stdout(stdout):
                    argv = [coin]
                    if always_build_backend:
                        argv = ["--always-build-backend", *argv]
                    build_packages.main(argv)
        finally:
            os.chdir(old_cwd)

        return commands[-1], stdout.getvalue().strip()

    def test_builds_backend_when_rpc_url_uses_localhost(self) -> None:
        make_cmd, output = self.run_build(
            coin="base_archive",
            rpc_env="BB_DEV_RPC_URL_HTTP_base_archive",
            rpc_url="http://localhost:18026",
            always_build_backend=False,
        )

        self.assertEqual(make_cmd, ["make", "deb-base_archive"])
        self.assertEqual(output, "build/blockbook-base_1.0_amd64.deb")
        staged_dir = self.package_root / "feature-test-branch" / "base_archive"
        self.assertTrue((staged_dir / "blockbook-base_1.0_amd64.deb").is_file())
        self.assertTrue((staged_dir / "backend-base_1.0_amd64.deb").is_file())

    def test_builds_backend_when_rpc_url_uses_loopback_ip(self) -> None:
        make_cmd, output = self.run_build(
            coin="base_archive",
            rpc_env="BB_DEV_RPC_URL_HTTP_base_archive",
            rpc_url="http://127.0.0.1:18026",
            always_build_backend=False,
        )

        self.assertEqual(make_cmd, ["make", "deb-base_archive"])
        self.assertEqual(output, "build/blockbook-base_1.0_amd64.deb")
        staged_dir = self.package_root / "feature-test-branch" / "base_archive"
        self.assertTrue((staged_dir / "blockbook-base_1.0_amd64.deb").is_file())
        self.assertTrue((staged_dir / "backend-base_1.0_amd64.deb").is_file())

    def test_skips_backend_when_rpc_url_host_is_remote(self) -> None:
        make_cmd, output = self.run_build(
            coin="base_archive",
            rpc_env="BB_DEV_RPC_URL_HTTP_base_archive",
            rpc_url="https://rpc.example.invalid/",
            always_build_backend=False,
        )

        self.assertEqual(make_cmd, ["make", "deb-blockbook-base_archive"])
        self.assertEqual(output, "build/blockbook-base_1.0_amd64.deb")
        staged_dir = self.package_root / "feature-test-branch" / "base_archive"
        self.assertTrue((staged_dir / "blockbook-base_1.0_amd64.deb").is_file())
        self.assertFalse((staged_dir / "backend-base_1.0_amd64.deb").exists())

    def test_skips_backend_when_localhost_only_appears_in_rpc_path(self) -> None:
        make_cmd, output = self.run_build(
            coin="base_archive",
            rpc_env="BB_DEV_RPC_URL_HTTP_base_archive",
            rpc_url="https://rpc.example.invalid/localhost",
            always_build_backend=False,
        )

        self.assertEqual(make_cmd, ["make", "deb-blockbook-base_archive"])
        self.assertEqual(output, "build/blockbook-base_1.0_amd64.deb")
        staged_dir = self.package_root / "feature-test-branch" / "base_archive"
        self.assertTrue((staged_dir / "blockbook-base_1.0_amd64.deb").is_file())
        self.assertFalse((staged_dir / "backend-base_1.0_amd64.deb").exists())

    def test_builds_backend_when_rpc_url_env_is_missing(self) -> None:
        make_cmd, output = self.run_build(
            coin="base_archive",
            always_build_backend=False,
        )

        self.assertEqual(make_cmd, ["make", "deb-base_archive"])
        self.assertEqual(output, "build/blockbook-base_1.0_amd64.deb")
        staged_dir = self.package_root / "feature-test-branch" / "base_archive"
        self.assertTrue((staged_dir / "blockbook-base_1.0_amd64.deb").is_file())
        self.assertTrue((staged_dir / "backend-base_1.0_amd64.deb").is_file())

    def test_builds_backend_when_rpc_url_env_is_empty(self) -> None:
        make_cmd, output = self.run_build(
            coin="base_archive",
            rpc_env="BB_DEV_RPC_URL_HTTP_base_archive",
            rpc_url="",
            always_build_backend=False,
        )

        self.assertEqual(make_cmd, ["make", "deb-base_archive"])
        self.assertEqual(output, "build/blockbook-base_1.0_amd64.deb")
        staged_dir = self.package_root / "feature-test-branch" / "base_archive"
        self.assertTrue((staged_dir / "blockbook-base_1.0_amd64.deb").is_file())
        self.assertTrue((staged_dir / "backend-base_1.0_amd64.deb").is_file())

    def test_skips_backend_when_rpc_url_env_is_non_empty_but_invalid(self) -> None:
        make_cmd, output = self.run_build(
            coin="base_archive",
            rpc_env="BB_DEV_RPC_URL_HTTP_base_archive",
            rpc_url="not-a-loopback-url",
            always_build_backend=False,
        )

        self.assertEqual(make_cmd, ["make", "deb-blockbook-base_archive"])
        self.assertEqual(output, "build/blockbook-base_1.0_amd64.deb")
        staged_dir = self.package_root / "feature-test-branch" / "base_archive"
        self.assertTrue((staged_dir / "blockbook-base_1.0_amd64.deb").is_file())
        self.assertFalse((staged_dir / "backend-base_1.0_amd64.deb").exists())

    def test_always_build_backend_overrides_localhost_detection(self) -> None:
        make_cmd, output = self.run_build(
            coin="base_archive",
            rpc_env="BB_DEV_RPC_URL_HTTP_base_archive",
            rpc_url="https://rpc.example.invalid/",
            always_build_backend=True,
        )

        self.assertEqual(make_cmd, ["make", "deb-base_archive"])
        self.assertEqual(output, "build/blockbook-base_1.0_amd64.deb")
        staged_dir = self.package_root / "feature-test-branch" / "base_archive"
        self.assertTrue((staged_dir / "backend-base_1.0_amd64.deb").is_file())

    def test_staging_uses_config_name_while_rpc_env_uses_alias(self) -> None:
        make_cmd, output = self.run_build(
            coin="polygon_archive",
            rpc_env="BB_DEV_RPC_URL_HTTP_polygon_archive_bor",
            rpc_url="http://localhost:8545",
            always_build_backend=False,
        )

        self.assertEqual(make_cmd, ["make", "deb-polygon_archive"])
        self.assertEqual(output, "build/blockbook-polygon_1.0_amd64.deb")
        staged_dir = self.package_root / "feature-test-branch" / "polygon_archive"
        alias_dir = self.package_root / "feature-test-branch" / "polygon_archive_bor"
        self.assertTrue((staged_dir / "blockbook-polygon_1.0_amd64.deb").is_file())
        self.assertTrue((staged_dir / "backend-polygon_1.0_amd64.deb").is_file())
        self.assertFalse(alias_dir.exists())

    def test_prod_build_env_uses_prod_rpc_url_prefix(self) -> None:
        make_cmd, output = self.run_build(
            coin="base_archive",
            build_env="prod",
            rpc_env="BB_PROD_RPC_URL_HTTP_base_archive",
            rpc_url="https://rpc.example.invalid/",
            always_build_backend=False,
        )

        self.assertEqual(make_cmd, ["make", "deb-blockbook-base_archive"])
        self.assertEqual(output, "build/blockbook-base_1.0_amd64.deb")
        staged_dir = self.package_root / "feature-test-branch" / "base_archive"
        self.assertTrue((staged_dir / "blockbook-base_1.0_amd64.deb").is_file())
        self.assertFalse((staged_dir / "backend-base_1.0_amd64.deb").exists())

    def test_prod_build_env_ignores_dev_rpc_url_prefix(self) -> None:
        make_cmd, output = self.run_build(
            coin="base_archive",
            build_env="prod",
            rpc_env="BB_DEV_RPC_URL_HTTP_base_archive",
            rpc_url="https://rpc.example.invalid/",
            always_build_backend=False,
        )

        self.assertEqual(make_cmd, ["make", "deb-base_archive"])
        self.assertEqual(output, "build/blockbook-base_1.0_amd64.deb")
        staged_dir = self.package_root / "feature-test-branch" / "base_archive"
        self.assertTrue((staged_dir / "blockbook-base_1.0_amd64.deb").is_file())
        self.assertTrue((staged_dir / "backend-base_1.0_amd64.deb").is_file())

    def test_fails_on_invalid_build_env(self) -> None:
        env = {
            "BRANCH_OR_TAG": "feature/test-branch",
            "BB_PACKAGE_ROOT": str(self.package_root),
            "BB_BUILD_ENV": "staging",
        }
        old_cwd = Path.cwd()
        try:
            os.chdir(self.workspace)
            with patch.dict(os.environ, env, clear=True), patch("build_packages.subprocess.run"):
                with self.assertRaises(SystemExit):
                    build_packages.main(["base_archive"])
        finally:
            os.chdir(old_cwd)


if __name__ == "__main__":
    unittest.main()
