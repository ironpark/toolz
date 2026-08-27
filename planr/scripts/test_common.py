from __future__ import annotations

import os
import pathlib
import tempfile
import unittest
from unittest import mock

from common import (
    HARNESS_CONFIG_PATH,
    HarnessError,
    force_remove_tree,
    load_harness_config,
    load_run_harness_config,
    build_tool,
    write_run_harness_config,
)


class ForceRemoveTreeTest(unittest.TestCase):
    def test_removes_read_only_files_like_the_go_module_cache(self) -> None:
        root = pathlib.Path(tempfile.mkdtemp())
        nested = root / "mod" / "example.com" / "pkg@v1.0.0"
        nested.mkdir(parents=True)
        (nested / "LICENSE").write_text("x", encoding="utf-8")
        os.chmod(nested / "LICENSE", 0o444)
        os.chmod(nested, 0o555)
        force_remove_tree(root)
        self.assertFalse(root.exists())

    def test_missing_path_is_not_an_error(self) -> None:
        force_remove_tree(pathlib.Path(tempfile.mkdtemp()) / "nope")


class HarnessConfigTest(unittest.TestCase):
    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.directory = pathlib.Path(directory.name)

    def test_source_config_is_valid_and_contains_all_four_seams(self) -> None:
        config = load_harness_config()
        self.assertEqual(HARNESS_CONFIG_PATH.name, "HARNESS.json")
        self.assertEqual(set(config), {"tool", "observe", "signals", "completion"})
        self.assertEqual(config["tool"]["name"], "planr")
        self.assertEqual(config["completion"]["command"], ["overview", "--json"])

    def test_resolved_config_round_trips_through_a_run_directory(self) -> None:
        config = load_harness_config()
        run_dir = self.directory / "run"
        run_dir.mkdir()
        written = write_run_harness_config(run_dir, config)
        self.assertEqual(written.name, "harness.json")
        self.assertEqual(load_run_harness_config(run_dir), config)

    def test_missing_config_names_the_file(self) -> None:
        path = self.directory / "missing.json"
        with self.assertRaisesRegex(HarnessError, str(path.resolve())):
            load_harness_config(path)

    def test_malformed_config_names_the_json_problem(self) -> None:
        path = self.directory / "bad.json"
        path.write_text("{\n", encoding="utf-8")
        with self.assertRaisesRegex(HarnessError, "invalid JSON"):
            load_harness_config(path)

    def test_incomplete_config_names_the_missing_section(self) -> None:
        path = self.directory / "incomplete.json"
        path.write_text("{}\n", encoding="utf-8")
        with self.assertRaisesRegex(HarnessError, r"root\.tool is required"):
            load_harness_config(path)

    def test_run_config_writer_rejects_an_incomplete_mapping(self) -> None:
        run_dir = self.directory / "run"
        run_dir.mkdir()
        config = load_harness_config()
        del config["tool"]["name"]
        with self.assertRaisesRegex(HarnessError, r"harness\.json.*tool\.name is required"):
            write_run_harness_config(run_dir, config)


class BuildToolTest(unittest.TestCase):
    def test_build_uses_the_configured_command_and_directory(self) -> None:
        config = load_harness_config()
        config_path = pathlib.Path(tempfile.mkdtemp()) / "HARNESS.json"
        self.addCleanup(lambda: force_remove_tree(config_path.parent))
        build_directory = config_path.parent / "source"
        build_directory.mkdir()
        config["tool"]["build"]["directory"] = "source"
        destination = config_path.parent / "workspace" / "bin" / "custom-tool"
        completed = mock.Mock(returncode=0, stdout="")
        with mock.patch("common.require_command") as require, mock.patch(
            "common.run_command", return_value=completed
        ) as run:
            build_tool(config, destination, config_path=config_path)
        require.assert_called_once_with("go")
        run.assert_called_once_with(
            ["go", "build", "-o", str(destination.resolve()), "."],
            cwd=build_directory.resolve(),
        )
        self.assertTrue(destination.parent.is_dir())


if __name__ == "__main__":
    unittest.main()
