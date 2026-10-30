"""Check source bootstrap boundaries without network, builds, or host configuration."""
import json
import os
from pathlib import Path
import subprocess
import shutil
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / 'scripts/bootstrap.sh'

class BootstrapTest(unittest.TestCase):
    def setUp(self):
        (ROOT / '.test-workspace').mkdir(exist_ok=True)
        self.tmp = tempfile.TemporaryDirectory(dir=ROOT / '.test-workspace', prefix='bootstrap.')
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.repo = self.base / 'personal project $(touch unwanted)'
        self.repo.mkdir()
        self.fake = self.base / 'tools'
        self.fake.mkdir()
        self.log = self.base / 'commands.jsonl'
        self.env = os.environ.copy()
        self.env.update(PATH=str(self.fake) + os.pathsep + self.env['PATH'], STEF_BOOTSTRAP_LOG=str(self.log), STEF_BOOTSTRAP_REAL_NODE=shutil.which('node'))
        self.wrapper = '''import json, os, pathlib, sys
args = sys.argv[1:]
name = pathlib.Path(sys.argv[0]).name
with open(os.environ['STEF_BOOTSTRAP_LOG'], 'a') as log: log.write(json.dumps([name, *args])+'\\n')
if name=='node':
 import subprocess
 code = "Object.defineProperty(process.versions,'node',{value:"+json.dumps(os.environ.get('STEF_BOOTSTRAP_NODE','22.12.0'))+"});"+args[1]
 raise SystemExit(subprocess.run([os.environ['STEF_BOOTSTRAP_REAL_NODE'],'-e',code]).returncode)
if name=='go': print(os.environ.get('STEF_BOOTSTRAP_GO','go version go1.25.0 test/arm64'))
if name=='git' and 'get-url' in args: print(os.environ.get('STEF_BOOTSTRAP_ORIGIN','https://github.com/ivandosevv/stef.git'))
if name=='git' and args[0]=='clone':
 source=pathlib.Path(args[-1]); (source/'.git').mkdir(parents=True); (source/'daemon').mkdir(); (source/'Makefile').touch(); (source/'daemon/go.mod').touch()
if name=='make' and args[-1]=='build':
 source=pathlib.Path(args[1]); (source/'bin').mkdir(exist_ok=True)
 launcher=source/'bin/stef'; launcher.write_text('#!'+sys.executable+'\\n'+''' + repr("import json, os, sys\nwith open(os.environ['STEF_BOOTSTRAP_LOG'],'a') as log: log.write(json.dumps(['stef',*sys.argv[1:]])+'\\n')\nprint('{}')\n") + '''); launcher.chmod(0o755)
'''
        for name in ['git', 'go', 'node', 'npm', 'make']:
            path = self.fake / name
            path.write_text('#!' + sys.executable + '\n' + self.wrapper)
            path.chmod(0o755)

    def run_bootstrap(self, *args, success=True):
        result = subprocess.run(['/bin/bash', str(SCRIPT), '--repo', str(self.repo), '--client', 'opencode', *args], env=self.env, text=True, capture_output=True)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0)
        return result

    def commands(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def checkout(self, path):
        (path / '.git').mkdir(parents=True)
        (path / 'daemon').mkdir()
        (path / 'Makefile').touch()
        (path / 'daemon/go.mod').touch()

    def test_first_setup_uses_project_local_source_and_exact_arguments(self):
        self.run_bootstrap()
        source = self.repo / '.stef/runtime/stef'
        commands = self.commands()
        self.assertIn(['git', 'clone', '--depth', '1', '--branch', 'master', 'https://github.com/ivandosevv/stef.git', str(source)], commands)
        self.assertIn(['make', '-C', str(source), 'install'], commands)
        self.assertIn(['make', '-C', str(source), 'build'], commands)
        self.assertEqual(commands[-1], ['stef', '--enable', '--client', 'opencode', '--view', 'ide', '--repo', str(self.repo)])
        self.assertFalse((self.base / 'unwanted').exists())

    def test_existing_checkout_never_pulls_or_overwrites_it(self):
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

    def test_build_only_never_configures_or_launches_a_companion(self):
        self.run_bootstrap('--build-only')
        self.assertFalse(any(command[0] == 'stef' for command in self.commands()))

    def test_non_checkout_and_foreign_origin_fail_before_build(self):
        source = self.base / 'occupied'
        source.mkdir()
        self.run_bootstrap('--source-dir', str(source), success=False)
        self.assertFalse(any(command[0] == 'make' for command in self.commands()))
        self.checkout(source)
        self.env['STEF_BOOTSTRAP_ORIGIN'] = 'https://example.invalid/another-repo'
        self.run_bootstrap('--source-dir', str(source), success=False)
        self.assertFalse(any(command[0] == 'make' for command in self.commands()))

    def test_default_source_refuses_a_project_symlink_escape(self):
        outside = self.base / 'outside'
        outside.mkdir()
        (self.repo / '.stef').symlink_to(outside, target_is_directory=True)
        result = self.run_bootstrap(success=False)
        self.assertIn('escapes the project', result.stderr)
        self.assertEqual(list(outside.iterdir()), [])
        self.assertEqual(self.commands(), [])

    def test_node_version_matches_vite_minimum_before_download(self):
        for version in ['20.19.0', '22.0.0', '22.11.0']:
            self.env['STEF_BOOTSTRAP_NODE'] = version
            result = self.run_bootstrap(success=False)
            self.assertIn('Node 22.12+', result.stderr)
            self.assertFalse(any(command[0] == 'git' for command in self.commands()))
        self.env['STEF_BOOTSTRAP_NODE'] = '22.12.0'
        self.run_bootstrap('--build-only')

    def test_old_go_fails_before_download(self):
        self.env['STEF_BOOTSTRAP_GO'] = 'go version go1.24.0 test/arm64'
        result = self.run_bootstrap(success=False)
        self.assertIn('Go 1.25+', result.stderr)
        self.assertFalse(any(command[0] == 'git' for command in self.commands()))
        self.assertFalse((self.repo / '.stef').exists())

    def test_relative_source_and_unknown_options_are_rejected(self):
        self.run_bootstrap('--source-dir', 'relative/path', success=False)
        self.run_bootstrap('--unknown', success=False)
        self.assertEqual(self.commands(), [])

if __name__ == '__main__':
    unittest.main()
