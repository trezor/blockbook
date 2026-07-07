import json
import unittest

from wait_for_sync import parse_sync_state


def encode_status(blockbook: dict, backend: dict | None = None) -> bytes:
    payload = {"blockbook": blockbook}
    if backend is not None:
        payload["backend"] = backend
    return json.dumps(payload).encode("utf-8")


class WaitForSyncTest(unittest.TestCase):
    def test_accepts_when_blockbook_is_synced_and_backend_lag_is_one(self) -> None:
        ready, summary = parse_sync_state(
            encode_status(
                {
                    "inSync": True,
                    "initialSync": False,
                    "bestHeight": 100,
                },
                {"blocks": 101},
            )
        )

        self.assertTrue(ready)
        self.assertIn("heightLag=1", summary)

    def test_rejects_when_initial_sync_is_true(self) -> None:
        ready, summary = parse_sync_state(
            encode_status(
                {
                    "inSync": True,
                    "initialSync": True,
                    "bestHeight": 100,
                },
                {"blocks": 100},
            )
        )

        self.assertFalse(ready)
        self.assertIn("initialSync=True", summary)

    def test_rejects_when_backend_height_gap_is_too_large(self) -> None:
        ready, summary = parse_sync_state(
            encode_status(
                {
                    "inSync": True,
                    "initialSync": False,
                    "bestHeight": 100,
                },
                {"blocks": 150},
            )
        )

        self.assertFalse(ready)
        self.assertIn("heightLag=50", summary)

    def test_preserves_existing_behavior_when_backend_height_is_missing(self) -> None:
        ready, summary = parse_sync_state(
            encode_status(
                {
                    "inSync": True,
                    "initialSync": False,
                    "bestHeight": 100,
                }
            )
        )

        self.assertTrue(ready)
        self.assertIn("backendBlocks=None", summary)


if __name__ == "__main__":
    unittest.main()
