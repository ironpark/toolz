from __future__ import annotations

import os
import pathlib
import tempfile
import unittest

from common import force_remove_tree


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


if __name__ == "__main__":
    unittest.main()
