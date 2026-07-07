import unittest

from admin_api_e2e import coin_deployed, parse_env_credentials, plan_settings_roundtrip


class ParseEnvCredentialsTest(unittest.TestCase):
    def test_plain(self) -> None:
        blob = "ETH_ALLOWED_RPC_CALL_TO=0xabc\nBB_ADMIN_USER=admin\nBB_ADMIN_PASSWORD=s3cr3t\n"
        self.assertEqual(parse_env_credentials(blob), ("admin", "s3cr3t"))

    def test_whitespace_and_cr(self) -> None:
        blob = "  BB_ADMIN_USER = admin \r\nBB_ADMIN_PASSWORD=s3cr3t \r\n"
        self.assertEqual(parse_env_credentials(blob), ("admin", "s3cr3t"))

    def test_value_containing_equals(self) -> None:
        blob = "BB_ADMIN_USER=admin\nBB_ADMIN_PASSWORD=pa=ss=word\n"
        self.assertEqual(parse_env_credentials(blob), ("admin", "pa=ss=word"))

    def test_surrounding_quotes_stripped_like_systemd(self) -> None:
        # systemd's EnvironmentFile strips one pair of quotes before the value
        # reaches the server, so the script must send the same unquoted value
        blob = "BB_ADMIN_USER=\"admin\"\nBB_ADMIN_PASSWORD='s3c r3t'\n"
        self.assertEqual(parse_env_credentials(blob), ("admin", "s3c r3t"))

    def test_inner_and_unmatched_quotes_kept(self) -> None:
        blob = "BB_ADMIN_USER=ad\"min\nBB_ADMIN_PASSWORD=\"s3cr3t\n"
        self.assertEqual(parse_env_credentials(blob), ('ad"min', '"s3cr3t'))

    def test_comments_and_missing_keys(self) -> None:
        blob = "# BB_ADMIN_USER=commented\nOTHER=x\n"
        self.assertEqual(parse_env_credentials(blob), ("", ""))


class CoinDeployedTest(unittest.TestCase):
    def test_empty_list_means_unrestricted(self) -> None:
        self.assertTrue(coin_deployed("", "base_archive"))
        self.assertTrue(coin_deployed("   ", "base_archive"))

    def test_membership(self) -> None:
        self.assertTrue(coin_deployed("bitcoin, base_archive ,zcash", "base_archive"))
        self.assertTrue(coin_deployed("Base_Archive", "base_archive"))
        self.assertFalse(coin_deployed("bitcoin,zcash", "base_archive"))


class PlanSettingsRoundtripTest(unittest.TestCase):
    """The planner must keep the parsed policy identical at every instant and
    end in the starting state (except db-sourced settings, which are only
    rewritten in place because their env fallback is unknown)."""

    def test_env_sourced(self) -> None:
        self.assertEqual(
            plan_settings_roundtrip("0xabc,0xdef", "env"),
            [
                ("POST", "0xabc,0xdef", "0xabc,0xdef", "db"),
                ("DELETE", None, "0xabc,0xdef", "env"),
            ],
        )

    def test_unset(self) -> None:
        self.assertEqual(
            plan_settings_roundtrip("", "unset"),
            [("POST", "", "", "db"), ("DELETE", None, "", "unset")],
        )

    def test_db_sourced_is_rewritten_in_place_only(self) -> None:
        self.assertEqual(
            plan_settings_roundtrip("0xabc", "db"),
            [("POST", "0xabc", "0xabc", "db")],
        )

    def test_every_step_preserves_the_effective_value(self) -> None:
        for value, source in (("0xabc", "env"), ("", "unset"), ("0xdd62ed3e", "db")):
            for _, _, expected_value, _ in plan_settings_roundtrip(value, source):
                self.assertEqual(expected_value, value, (value, source))

    def test_end_state_matches_start_state(self) -> None:
        for value, source in (("0xabc", "env"), ("", "unset")):
            _, _, final_value, final_source = plan_settings_roundtrip(value, source)[-1]
            self.assertEqual((final_value, final_source), (value, source))

    def test_unknown_source_raises(self) -> None:
        with self.assertRaises(ValueError):
            plan_settings_roundtrip("x", "bogus")


if __name__ == "__main__":
    unittest.main()
