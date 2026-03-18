import tempfile
import unittest
from pathlib import Path

from runner import (
    ValidationError,
    load_coin_context,
    resolve_build_selection,
    resolve_deploy_selection,
)


def write_text(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


class RunnerSelectionTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.workspace = Path(self.tempdir.name)

        write_text(
            self.workspace / "configs" / "coins" / "dogecoin.json",
            '{"coin":{"name":"Dogecoin"}}',
        )
        write_text(
            self.workspace / "configs" / "coins" / "base_archive.json",
            '{"coin":{"test_name":"base"}}',
        )
        write_text(
            self.workspace / "configs" / "coins" / "polygon_archive.json",
            '{"coin":{"test_name":"polygon"}}',
        )
        write_text(
            self.workspace / "tests" / "tests.json",
            '{"dogecoin":{"connectivity":{}},"base":{"connectivity":{}},"polygon":{"connectivity":{}}}',
        )

        self.valid_vars_map = {
            "BB_RUNNER_DOGECOIN": "blockbook-dev",
            "BB_RUNNER_BASE_ARCHIVE": "blockbook-dev3",
            "BB_RUNNER_POLYGON_ARCHIVE": "production_builder",
        }
        self.stale_vars_map = {
            **self.valid_vars_map,
            "BB_RUNNER_STALE": "blockbook-dev2",
        }

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def test_load_coin_context_rejects_runner_mapping_without_config(self) -> None:
        with self.assertRaisesRegex(
            ValidationError,
            r"BB_RUNNER_\* entries without matching configs/coins/<coin>\.json: stale ",
        ):
            load_coin_context(self.workspace, self.stale_vars_map)

    def test_build_all_uses_all_configured_runner_mapped_coins(self) -> None:
        context = load_coin_context(self.workspace, self.valid_vars_map)

        selection = resolve_build_selection(context, "ALL", "prod")

        self.assertEqual(
            selection.coins,
            ["base_archive", "dogecoin", "polygon_archive"],
        )

    def test_build_dev_rejects_explicit_prod_only_coin(self) -> None:
        context = load_coin_context(self.workspace, self.valid_vars_map)

        with self.assertRaisesRegex(
            ValidationError,
            "coin not available in build env=dev: polygon_archive",
        ):
            resolve_build_selection(context, "polygon_archive", "dev")

    def test_build_dev_all_skips_prod_only_coins(self) -> None:
        context = load_coin_context(self.workspace, self.valid_vars_map)

        selection = resolve_build_selection(context, "ALL", "dev")

        self.assertEqual(selection.coins, ["base_archive", "dogecoin"])
        self.assertEqual(selection.skipped_prod_only, ["polygon_archive"])

    def test_deploy_all_lists_deployable_coins(self) -> None:
        context = load_coin_context(self.workspace, self.valid_vars_map, include_deployability=True)

        with self.assertRaisesRegex(
            ValidationError,
            "deploy does not support ALL; deployable coins: base_archive,dogecoin",
        ):
            resolve_deploy_selection(context, "ALL")

    def test_deploy_rejects_prod_only_coin_with_reason(self) -> None:
        context = load_coin_context(self.workspace, self.valid_vars_map, include_deployability=True)

        with self.assertRaisesRegex(
            ValidationError,
            "coin 'polygon_archive' is not deployable in dev",
        ):
            resolve_deploy_selection(context, "polygon_archive")


if __name__ == "__main__":
    unittest.main()
