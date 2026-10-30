"""Check the source setup entry without network, real builds or client profiles."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / 'scripts/bootstrap.sh'
LAUNCHER = r"""import json, os, sys
with open(os.environ['STEF_BOOTSTRAP_LOG'], 'a') as log:
 log.write(json.dumps(['stef', *sys.argv[1:]]) + '\n')
print(json.dumps({'client_configured': False, 'mcp_connected': False, 'view_opened': False}))
"""


@unittest.skipIf(os.name == 'nt', 'The compatibility Bash entry is exercised on macOS and Linux.')
class BootstrapTest(unittest.TestCase):
    def setUp(self):
        (ROOT / '.test-workspace').mkdir(exist_ok=True)
        self.tmp = tempfile.TemporaryDirectory(dir=ROOT / '.test-workspace', prefix='bootstrap.')
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.repo = self.base / 'personal project $(touch unwanted) & spaces'
        self.repo.mkdir()
        self.fake = self.base / 'tools'
        self.fake.mkdir()
        self.log = self.base / 'commands.jsonl'
        self.env = os.environ.copy()
        self.env.update(PATH=str(self.fake) + os.pathsep + self.env['PATH'],
                        STEF_BOOTSTRAP_LOG=str(self.log), STEF_BOOTSTRAP_ROOT=str(ROOT))
        wrapper = f"""import json, os, pathlib, shutil, sys
args = sys.argv[1:]
name = pathlib.Path(sys.argv[0]).name
with open(os.environ['STEF_BOOTSTRAP_LOG'], 'a') as log:
 log.write(json.dumps([name, *args]) + '\\n')
if name == 'node': print(os.environ.get('STEF_BOOTSTRAP_NODE', 'v22.12.0'))
if name == 'go' and args == ['version']:
 print(os.environ.get('STEF_BOOTSTRAP_GO', 'go version go1.25.0 test/arm64'))
if name == 'git' and 'get-url' in args:
 print(os.environ.get('STEF_BOOTSTRAP_ORIGIN', 'https://github.com/ivandosevv/stef.git'))
if name == 'git' and args[0] == 'clone':
 source = pathlib.Path(args[-1])
 for folder in ['.git', 'daemon', 'daemon/assets', 'ui', 'scripts']: (source / folder).mkdir(parents=True, exist_ok=True)
 (source / 'daemon/go.mod').touch()
 (source / 'ui/package-lock.json').touch()
 (source / 'LICENSE').write_text('MIT fixture')
 for script in ['build.py', 'embed-ui.py']:
  shutil.copy(pathlib.Path(os.environ['STEF_BOOTSTRAP_ROOT']) / 'scripts' / script, source / 'scripts' / script)
if name == 'npm' and args == ['run', 'build']:
 source = pathlib.Path.cwd(); (source / 'dist').mkdir(exist_ok=True)
 (source / 'dist/index.html').write_text('<html>fixture</html>')
if name == 'go' and args[0] == 'build':
 launcher = pathlib.Path(args[args.index('-o') + 1])
 launcher.write_text('#!' + sys.executable + '\\n' + {LAUNCHER!r})
 launcher.chmod(0o755)
 with open(os.environ['STEF_BOOTSTRAP_LOG'], 'a') as log:
  log.write(json.dumps(['compiler_env', os.environ.get('CGO_ENABLED'), os.environ.get('GOOS')]) + '\\n')
"""
        for name in ['git', 'go', 'node', 'npm']:
            path = self.fake / name
            path.write_text('#!' + sys.executable + '\n' + wrapper)
            path.chmod(0o755)

    def run_bootstrap(self, *args, success=True):
        result = subprocess.run(['/bin/bash', str(SCRIPT), '--repo', str(self.repo), '--client', 'opencode', *args],
                                env=self.env, text=True, capture_output=True)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0)
        return result

    def commands(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def checkout(self, path):
        for folder in ['.git', 'daemon', 'daemon/assets', 'ui', 'scripts']:
            (path / folder).mkdir(parents=True, exist_ok=True)
        (path / 'daemon/go.mod').touch()
        (path / 'ui/package-lock.json').touch()
        (path / 'LICENSE').write_text('MIT fixture')
        for script in ['build.py', 'embed-ui.py']:
            shutil.copy(ROOT / 'scripts' / script, path / 'scripts' / script)

    def test_source_setup_stays_project_local_with_literal_arguments(self):
        result = self.run_bootstrap()
        source = self.repo / '.stef/runtime/stef'
        commands = self.commands()
        self.assertIn(['git', 'clone', '--depth', '1', '--branch', 'master', 'https://github.com/ivandosevv/stef.git', str(source)], commands)
        self.assertIn(['npm', 'ci'], commands)
        self.assertIn(['npm', 'run', 'build'], commands)
        self.assertIn(['go', 'mod', 'download'], commands)
        self.assertIn(['compiler_env', '0', 'darwin' if sys.platform == 'darwin' else 'linux'], commands)
        self.assertEqual(commands[-1], ['stef', '--enable', '--client', 'opencode', '--view', 'ide', '--repo', str(self.repo)])
        receipt = json.loads(result.stdout)
        self.assertEqual(receipt['executable'], str(self.repo / '.stef/runtime/bin/stef'))
        self.assertFalse(receipt['mcp_connected'])
        self.assertFalse(any(command[0] == 'make' for command in commands))
        self.assertFalse((self.base / 'unwanted').exists())

    def test_existing_checkout_reuses_edits_and_never_pulls(self):
        source = self.base / 'stef source'
        self.checkout(source)
        marker = source / 'my-edit.txt'
        marker.write_text('keep')
        self.run_bootstrap('--source-dir', str(source), '--view', 'native')
        commands = self.commands()
        self.assertFalse(any(command[0] == 'git' and 'clone' in command for command in commands))
        self.assertFalse(any('pull' in command or 'reset' in command for command in commands))
        self.assertEqual(marker.read_text(), 'keep')
        self.assertEqual(commands[-1][commands[-1].index('--view') + 1], 'native')

    def test_build_only_never_configures_or_launches(self):
        self.run_bootstrap('--build-only')
        self.assertFalse(any(command[0] == 'stef' for command in self.commands()))

    def test_foreign_or_occupied_source_fails_before_build(self):
        source = self.base / 'occupied'
        source.mkdir()
        self.run_bootstrap('--source-dir', str(source), success=False)
        self.assertFalse(any(command[0] == 'npm' for command in self.commands()))
        self.checkout(source)
        self.env['STEF_BOOTSTRAP_ORIGIN'] = 'https://example.invalid/another-repo'
        self.run_bootstrap('--source-dir', str(source), success=False)
        self.assertFalse(any(command[0] == 'npm' for command in self.commands()))

    def test_symlink_escape_fails_before_commands_or_download(self):
        outside = self.base / 'outside'
        outside.mkdir()
        (self.repo / '.stef').symlink_to(outside, target_is_directory=True)
        result = self.run_bootstrap(success=False)
        self.assertIn('escapes the project', result.stderr)
        self.assertEqual(list(outside.iterdir()), [])
        self.assertEqual(self.commands(), [])

    def test_old_tool_versions_fail_before_cloning_or_building(self):
        for version in ['v20.19.0', 'v22.0.0', 'v22.11.0']:
            self.env['STEF_BOOTSTRAP_NODE'] = version
            result = self.run_bootstrap(success=False)
            self.assertIn('Node 22.12+', result.stderr)
            self.assertFalse(any(command[0] == 'git' for command in self.commands()))
        self.env['STEF_BOOTSTRAP_NODE'] = 'v22.12.0'
        self.env['STEF_BOOTSTRAP_GO'] = 'go version go1.24.0 test/arm64'
        result = self.run_bootstrap(success=False)
        self.assertIn('Go 1.25+', result.stderr)
        self.assertFalse((self.repo / '.stef').exists())

    def test_relative_source_unknown_flags_and_home_are_rejected(self):
        self.run_bootstrap('--source-dir', 'relative/path', success=False)
        self.run_bootstrap('--unknown', success=False)
        self.run_bootstrap('--repo', str(Path.home()), success=False)
        self.assertEqual(self.commands(), [])


if __name__ == '__main__':
    unittest.main()
