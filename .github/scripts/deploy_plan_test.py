import json
import re
import unittest
from pathlib import Path

from deploy_plan import build_connectivity_regex, matchable_name

REPO_ROOT = Path(__file__).resolve().parents[2]


class MatchableNameTest(unittest.TestCase):
    def test_known_mappings(self) -> None:
        cases = {
            "ethereum": "ethereum=main",
            "tron": "tron=main",
            "bitcoin_regtest": "bitcoin_regtest=main",
            "bitcoin_testnet": "bitcoin=test",
            "bitcoin_testnet4": "bitcoin=test4",
            "ethereum_testnet_sepolia": "ethereum=test_sepolia",
            "ethereum_testnet_hoodi": "ethereum=test_hoodi",
            "tron_testnet_nile": "tron=test_nile",
        }
        for coin, expected in cases.items():
            self.assertEqual(matchable_name(coin), expected, coin)

    def test_sibling_testnets_do_not_collide(self) -> None:
        # The bug this guards: every "<coin>_testnet*" used to collapse to
        # "<coin>=test", so the deploy connectivity regex for one testnet also
        # selected its siblings.
        self.assertNotEqual(
            matchable_name("ethereum_testnet_sepolia"),
            matchable_name("ethereum_testnet_hoodi"),
        )
        self.assertNotEqual(
            matchable_name("bitcoin_testnet"),
            matchable_name("bitcoin_testnet4"),
        )

    def test_injective_over_real_tests_json(self) -> None:
        # Regression guard: no two tests.json keys may share a matchable name,
        # otherwise the deploy connectivity gate would run sibling coins too.
        keys = json.loads((REPO_ROOT / "tests" / "tests.json").read_text("utf-8")).keys()
        seen: dict[str, str] = {}
        for key in keys:
            name = matchable_name(key)
            self.assertNotIn(
                name, seen, f"{key} and {seen.get(name)} both map to {name}"
            )
            seen[name] = key


class ConnectivityRegexTest(unittest.TestCase):
    def _element_pattern(self, regex: str) -> str:
        # The middle "/(...)/" group is what `go test -run` matches against a
        # single subtest-name path element.
        match = re.fullmatch(r"TestIntegration/\((.*)\)/connectivity", regex)
        self.assertIsNotNone(match, regex)
        return match.group(1)

    def test_anchored_names_do_not_substring_match_siblings(self) -> None:
        pattern = self._element_pattern(build_connectivity_regex(["bitcoin=test"]))
        # Unanchored, "bitcoin=test" would also match "bitcoin=test4"; anchored it
        # must not.
        self.assertTrue(re.search(pattern, "bitcoin=test"))
        self.assertFalse(re.search(pattern, "bitcoin=test4"))

    def test_multiple_names_are_alternated(self) -> None:
        pattern = self._element_pattern(
            build_connectivity_regex(["ethereum=test_hoodi", "ethereum=test_sepolia"])
        )
        self.assertTrue(re.search(pattern, "ethereum=test_hoodi"))
        self.assertTrue(re.search(pattern, "ethereum=test_sepolia"))
        self.assertFalse(re.search(pattern, "ethereum=main"))

    def test_empty_names_fails_closed(self) -> None:
        # An empty alternation "()" would match every connectivity subtest, the
        # opposite of selecting no coins; the function must refuse instead.
        with self.assertRaises(ValueError):
            build_connectivity_regex([])


if __name__ == "__main__":
    unittest.main()
