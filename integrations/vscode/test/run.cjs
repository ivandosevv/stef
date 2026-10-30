const { runTests, downloadAndUnzipVSCode, resolveCliArgsFromVSCodeExecutablePath } = require('@vscode/test-electron');
const { resolve, join } = require('node:path');
const { mkdir, mkdtemp, copyFile, writeFile, readdir, rm } = require('node:fs/promises');
const { existsSync } = require('node:fs');
const { execFileSync } = require('node:child_process');

(async () => {
  delete process.env.ELECTRON_RUN_AS_NODE;
  const project = resolve('../..');
  await mkdir(join(project, '.test-workspace'), { recursive: true });
  const root = await mkdtemp(join(project, '.test-workspace', 'ide.'));
  execFileSync('git', ['init', '-q', root]);
  await copyFile(join(project, 'examples/sandbox/main.go'), join(root, 'main.go'));
  await writeFile(join(root, '.gitignore'), ['profile/', 'extensions/', '.stef/', ''].join(String.fromCharCode(10)));
  const installed = '/Applications/Visual Studio Code 2.app/Contents/MacOS/Code';
  const executable = process.env.VSCODE_EXECUTABLE_PATH || (existsSync(installed) ? installed : await downloadAndUnzipVSCode({ version: 'stable', cachePath: join(project, '.test-workspace/vscode-cache') }));
  try {
    let extensionPath = process.env.STEF_EXTENSION_PATH || resolve('.');
    if (process.env.STEF_VSIX_PATH) {
      const [cli, ...args] = resolveCliArgsFromVSCodeExecutablePath(executable);
      execFileSync(cli, [...args, '--install-extension', process.env.STEF_VSIX_PATH, '--extensions-dir', join(root, 'extensions'), '--user-data-dir', join(root, 'profile'), '--force']);
      const folder = (await readdir(join(root, 'extensions'))).find((name) => name.startsWith('ivandosevv.stef-'));
      if (!folder) throw new Error('The stef VSIX was not installed into the isolated profile.');
      extensionPath = join(root, 'extensions', folder);
    }
    await runTests({
      extensionDevelopmentPath: extensionPath, extensionTestsPath: resolve('test/suite.cjs'),
      vscodeExecutablePath: executable,
      launchArgs: [root, '--disable-workspace-trust', '--disable-extensions', '--user-data-dir', join(root, 'profile'), '--extensions-dir', join(root, 'extensions'), '--skip-welcome', '--skip-release-notes'],
      extensionTestsEnv: { STEF_TEST_ROOT: root },
    });
  } finally { await rm(root, { recursive: true, force: true }); }
})().catch((error) => { console.error(error); process.exit(1); });
