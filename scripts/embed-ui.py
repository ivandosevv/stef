"""Copy only built assets into the Go embed directory, within the personal repo."""
from pathlib import Path
import shutil

root = Path(__file__).resolve().parent.parent
source = root / "ui" / "dist"
if not (source / "index.html").is_file():
    raise SystemExit("Build ui/dist before embedding")
target = root / "daemon" / "assets"
shutil.rmtree(target)
shutil.copytree(source, target)
(target / "placeholder.txt").write_text("Run make build to embed the stef UI.\n")
