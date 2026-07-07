#!/usr/bin/env python3
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(__file__))

import render_grafana


class GrafanaLayoutTest(unittest.TestCase):
    def test_compute_grid_positions_packs_left_to_right(self):
        items = [
            {"key": "row.websocket", "type": "row", "w": 24, "h": 1},
            {"key": "websocket.clients", "type": "timeseries", "w": 8, "h": 8},
            {"key": "websocket.requests", "type": "timeseries", "w": 8, "h": 8},
            {"key": "websocket.subscriptions", "type": "timeseries", "w": 8, "h": 8},
            {"key": "websocket.rejections", "type": "timeseries", "w": 12, "h": 8},
        ]

        positions, problems = render_grafana.compute_grid_positions(items)

        self.assertEqual([], problems)
        self.assertEqual(
            [
                {"h": 1, "w": 24, "x": 0, "y": 0},
                {"h": 8, "w": 8, "x": 0, "y": 1},
                {"h": 8, "w": 8, "x": 8, "y": 1},
                {"h": 8, "w": 8, "x": 16, "y": 1},
                {"h": 8, "w": 12, "x": 0, "y": 9},
            ],
            positions,
        )

    def test_compute_grid_positions_wraps_by_tallest_panel_in_row(self):
        items = [
            {"key": "left", "type": "timeseries", "w": 12, "h": 4},
            {"key": "right", "type": "timeseries", "w": 12, "h": 8},
            {"key": "next", "type": "timeseries", "w": 8, "h": 4},
        ]

        positions, problems = render_grafana.compute_grid_positions(items)

        self.assertEqual([], problems)
        self.assertEqual(
            [
                {"h": 4, "w": 12, "x": 0, "y": 0},
                {"h": 8, "w": 12, "x": 12, "y": 0},
                {"h": 4, "w": 8, "x": 0, "y": 8},
            ],
            positions,
        )

    def test_layout_items_reads_sizes_from_panels_yaml_with_defaults(self):
        # a row and a default-sized panel omit width/height; a wide panel overrides width.
        panels = [
            {"x-panel-key": "row.general", "type": "row"},
            {"x-panel-key": "general.tip_age", "type": "timeseries"},
            {"x-panel-key": "websocket.unique_ips", "type": "timeseries"},
        ]
        content = {"websocket.unique_ips": {"width": 12, "title": "..."}}

        items, problems = render_grafana.layout_items_from_panels(panels, content)

        self.assertEqual([], problems)
        self.assertEqual(
            [
                {"key": "row.general", "type": "row", "h": 1, "w": 24},
                {"key": "general.tip_age", "type": "timeseries", "h": 8, "w": 8},
                {"key": "websocket.unique_ips", "type": "timeseries", "h": 8, "w": 12},
            ],
            items,
        )
        # side-effect-free: layout extraction must not stamp gridPos onto the template panels
        self.assertNotIn("gridPos", panels[1])

    def test_layout_items_rejects_committed_gridpos(self):
        panels = [
            {
                "x-panel-key": "general.tip_age",
                "type": "timeseries",
                "gridPos": {"h": 8, "w": 8, "x": 0, "y": 1},
            },
        ]

        _, problems = render_grafana.layout_items_from_panels(panels, {})

        self.assertEqual(
            [
                "template panel general.tip_age must not carry gridPos; set width/height in panels.yaml "
                "(x/y are computed by render_grafana.py)",
            ],
            problems,
        )

    def test_layout_items_rejects_oversized_width(self):
        panels = [{"x-panel-key": "wide", "type": "timeseries"}]
        content = {"wide": {"width": 25}}

        _, problems = render_grafana.layout_items_from_panels(panels, content)

        self.assertEqual(["panel wide width=25 exceeds Grafana's 24-column grid"], problems)

    def test_apply_computed_grid_positions_packs_mixed_widths(self):
        panels = [
            {"x-panel-key": "general.synchronized", "type": "stat"},
            {"x-panel-key": "general.block_height", "type": "stat"},
            {"x-panel-key": "websocket.unique_ips", "type": "timeseries"},
        ]
        content = {"websocket.unique_ips": {"width": 12}}

        problems = render_grafana.apply_computed_grid_positions(panels, content)

        self.assertEqual([], problems)
        self.assertEqual({"h": 8, "w": 8, "x": 0, "y": 0}, panels[0]["gridPos"])
        self.assertEqual({"h": 8, "w": 8, "x": 8, "y": 0}, panels[1]["gridPos"])
        # 8 + 8 + 12 > 24, so the wide panel wraps to the next shelf
        self.assertEqual({"h": 8, "w": 12, "x": 0, "y": 8}, panels[2]["gridPos"])


def _valid_rendered_dash():
    ds = {"type": "prometheus", "uid": "${DS_PROMETHEUS}"}
    return {
        "schemaVersion": 41,
        "__inputs": [{"name": "DS_PROMETHEUS", "type": "datasource"}],
        "templating": {"list": [{"name": "coin", "type": "custom"}]},
        "panels": [
            {"id": 1, "type": "row", "collapsed": False, "panels": [],
             "gridPos": {"h": 1, "w": 24, "x": 0, "y": 0}},
            {"id": 2, "type": "timeseries", "datasource": ds,
             "gridPos": {"h": 8, "w": 8, "x": 0, "y": 1},
             "targets": [{"refId": "A", "datasource": ds, "expr": 'up{coin="$coin"}'}]},
            {"id": 3, "type": "timeseries", "datasource": ds,
             "gridPos": {"h": 8, "w": 8, "x": 8, "y": 1},
             "targets": [{"refId": "A", "datasource": ds, "expr": "up"}]},
        ],
    }


class GrafanaImportValidityTest(unittest.TestCase):
    def test_valid_dashboard_has_no_problems(self):
        self.assertEqual([], render_grafana.validate_rendered(_valid_rendered_dash()))

    def test_duplicate_panel_id_is_rejected(self):
        dash = _valid_rendered_dash()
        dash["panels"][2]["id"] = 2
        self.assertTrue(any("shared by" in p for p in render_grafana.validate_rendered(dash)))

    def test_overlapping_panels_are_rejected(self):
        dash = _valid_rendered_dash()
        dash["panels"][2]["gridPos"]["x"] = 0  # now sits on top of panel id 2
        self.assertTrue(any("overlap" in p for p in render_grafana.validate_rendered(dash)))

    def test_panel_past_the_grid_is_rejected(self):
        dash = _valid_rendered_dash()
        dash["panels"][2]["gridPos"]["x"] = 20  # 20 + 8 > 24
        self.assertTrue(any("past the 24-column grid" in p for p in render_grafana.validate_rendered(dash)))

    def test_target_without_datasource_is_rejected(self):
        dash = _valid_rendered_dash()
        dash["panels"][1]["targets"][0].pop("datasource")
        problems = render_grafana.validate_rendered(dash)
        self.assertTrue(any("target" in p and "datasource" in p for p in problems))

    def test_undeclared_input_variable_is_rejected(self):
        dash = _valid_rendered_dash()
        dash["__inputs"] = []  # ${DS_PROMETHEUS} now resolves to nothing
        self.assertTrue(any("DS_PROMETHEUS" in p for p in render_grafana.validate_rendered(dash)))

    def test_row_without_panels_list_is_rejected(self):
        dash = _valid_rendered_dash()
        dash["panels"][0].pop("panels")
        self.assertTrue(any("'panels' list" in p for p in render_grafana.validate_rendered(dash)))


if __name__ == "__main__":
    unittest.main()
