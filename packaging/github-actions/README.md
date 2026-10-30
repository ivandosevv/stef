# GitHub Actions template

check.yml preserves the full headless build, backend/UI checks, and browser test
workflow. It is stored as a template because the GitHub login available for the
first source checkpoint cannot publish files under .github/workflows without
the workflow permission. No hosted checks run from this location.

To activate CI using a Git login with permission to publish workflows:

~~~sh
mkdir -p .github/workflows
git mv packaging/github-actions/check.yml .github/workflows/check.yml
git add .github/workflows/check.yml packaging/github-actions
git commit -m "Enable GitHub Actions checks"
git push
~~~

Until activation is verified, run make check, make test, and the browser tests
locally as documented in the main README.
