"""Copy built assets into the VSIX without a runtime development server."""
from pathlib import Path
import shutil

root = Path(__file__).resolve().parent.parent
resources = root / "integrations/vscode/resources"
if resources.exists():
    shutil.rmtree(resources)
shutil.copytree(root / "ui/dist", resources / "ui")
(resources / "bin").mkdir()
shutil.copy2(root / "bin/stefd", resources / "bin/stefd")
shutil.copy2(root / "LICENSE", root / "integrations/vscode/LICENSE")
(root / "artifacts").mkdir(exist_ok=True)
