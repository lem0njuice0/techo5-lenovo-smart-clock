import contextlib
import hashlib
import io
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import cube_install


class InstallWrapperTests(unittest.TestCase):
    def test_install_requires_trial_confirmation_before_calling_fastboot(self):
        args = ['cube_install.py', '--work', 'missing', '--serial', 'clock', '--slot', 'a', '--action', 'install']
        with mock.patch('sys.argv', args), mock.patch('cube_install.subprocess.run') as run:
            with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(SystemExit):
                cube_install.main()
            run.assert_not_called()

    def test_trial_delegates_with_matching_slot_and_report(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'boot-a.img').write_bytes(b'boot fixture')
            args = ['cube_install.py', '--work', str(root), '--serial', 'clock', '--slot', 'a', '--action', 'trial']
            with mock.patch('sys.argv', args), mock.patch('cube_install.subprocess.run') as run:
                cube_install.main()
            cmd = run.call_args.args[0]
            self.assertEqual(cmd[cmd.index('--slot') + 1], 'a')
            self.assertEqual(cmd[cmd.index('--action') + 1], 'trial')
            self.assertEqual(cmd[cmd.index('--boot-sha256') + 1], hashlib.sha256(b'boot fixture').hexdigest())
            self.assertEqual(cmd[cmd.index('--root-report') + 1], str(root / 'root-report.json'))


if __name__ == '__main__':
    unittest.main()
