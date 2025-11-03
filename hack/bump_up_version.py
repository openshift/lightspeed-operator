#!/usr/bin/python

"""Bump-up service version on all places."""

"""
Usage:
    ./bump_up_version.py <current-version> <new-version>

Example:
    ./bump_up_version.py 0.2.1 0.2.2

The current version should be specified ATM to avoid skipping versions etc.
"""

import logging
import subprocess
import sys
from pathlib import Path

import semantic_version

logger = logging.getLogger("bump-up versions")


repositories = {
    "lightspeed-console": "version_patches/console.patch",
    "lightspeed-service": "version_patches/service.patch",
    "lightspeed-operator": "version_patches/operator.patch",
    "konflux-release-data": "version_patches/konflux-release-data.patch",
}


def check_cli_args(args: list[str]) -> None:
    """Check CLI arguments."""
    if len(args) != 3:
        logger.error("Usage: python bump_up_version.py <current-version> <new-version>")
        sys.exit(1)


def check_version_format(version: str) -> None:
    """Check that the provided input contains semantic version."""
    # at this moment this simple check is sufficient
    semantic_version.Version(version)


def read_current_version() -> str:
    """Read current service version."""
    completed = subprocess.run(  # noqa: S603
        ["pdm", "show", "--version"], capture_output=True, check=True  # noqa: S607
    )
    return completed.stdout.decode("utf-8").strip()


def read_template(patch: str) -> str:
    """Read patch template from file."""
    path = Path(__file__).parent / patch
    with open(path, "r") as fin:
        return fin.read()


def write_patch(filename: str, content: str) -> None:
    """Write patch to a file."""
    logger.info(f"creating patch file: {filename}")
    with open(filename, "w") as fout:
        fout.write(content)


def patch_name(repository: str, current_version: str, new_version: str) -> str:
    """Generate unique patch name."""
    return f"{repository}-bump-from-{current_version}-to-{new_version}.patch"


def prepare_patches(
    repositories: dict[str, str], current_version: str, new_version: str
) -> None:
    """Prepare patches for all repositories."""
    logger.info("started patches generation")
    for repository, patch in repositories.items():
        logger.info(f"preparing patch for repository '{repository}'")
        template = read_template(patch)
        content = template.replace("<current-version>", current_version).replace(
            "<new-version>", new_version
        )
        write_patch(patch_name(repository, current_version, new_version), content)
    logger.info("finished patches generation")


def check_return_code(result: subprocess.CompletedProcess, error_message: str) -> None:
    """Check if the command finished with ok status."""
    if result.returncode != 0:
        error = result.stderr.decode("utf-8")
        logger.error(error_message)
        logger.error(error)
        raise Exception(error)


def git_command(target_repository: str, *arguments: str) -> subprocess.CompletedProcess:
    """Perform selected GIT command with checks enabled."""
    return subprocess.run(  # noqa: S603
        ["git", "-C", target_repository, *arguments],  # noqa: S607
        capture_output=True,
        check=False,
    )


def sync_repository(target_repository: str) -> None:
    """Sync the local repository with origin."""
    result = git_command(target_repository, "checkout", "main")
    check_return_code(result, "git checkout to main branch failed")
    logger.info("checkout to main branch: ok")

    result = git_command(target_repository, "fetch", "origin")
    check_return_code(result, "git fetch failed")
    logger.info("fetch from origin repository: ok")

    result = git_command(target_repository, "rebase", "origin/main")
    check_return_code(result, "git rebase to origin/main failed")
    logger.info("rebase to origin/main: ok")
    logger.info("sync local repository finished")


def create_branch(target_repository: str, branch_name: str) -> None:
    """Create new branch for pull request with version bump-up."""
    result = git_command(target_repository, "checkout", "-b", branch_name)
    check_return_code(result, f"cannot create branch named {branch_name}")
    logger.info(f"branch {branch_name} has been created")


def check_if_applicable(target_repository: str, patch_file: str) -> None:
    """Check that the patch can be applied."""
    result = git_command(target_repository, "apply", "--check", "../" + patch_file)
    check_return_code(result, f"cannot apply patch {patch_file} (dry run)")
    logger.info("check if patch is applicable: ok")


def apply_patch(target_repository: str, patch_file: str) -> None:
    """Apply the patch to target repository."""
    result = git_command(target_repository, "apply", "../" + patch_file)
    check_return_code(result, f"cannot apply patch {patch_file} (real apply)")
    logger.info("apply patch: ok")


def commit_changes(target_repository: str, new_version: str) -> None:
    """Commit all changes into newly created branch."""
    result = git_command(target_repository, "add", ".")
    check_return_code(
        result, f"cannot add changes into reporitory {target_repository}"
    )
    result = git_command(target_repository, "commit", "-m", f"Bump to version {new_version}")
    check_return_code(
        result, f"cannot commit changes into reporitory {target_repository}"
    )
    logger.info("commit changes: ok")


def push_pr(target_repository: str, branch_name: str) -> None:
    """Push the pull request."""
    result = git_command(target_repository, "push", "origin", branch_name)
    check_return_code(result, "cannot push the pull request")
    logger.info("push the pull request: ok")


def prepare_pull_request(
    target_repository: str, current_version: str, new_version: str
) -> None:
    """Prepare pull request for given repository."""
    branch_name = f"bump-from-{current_version}-to-{new_version}"
    patch = patch_name(target_repository, current_version, new_version)
    #sync_repository(target_repository)
    #create_branch(target_repository, branch_name)
    #check_if_applicable(target_repository, patch)
    #apply_patch(target_repository, patch)
    commit_changes(target_repository, new_version)
    #push_pr(target_repository, branch_name)


def prepare_pull_requests(
    repositories: dict[str, str], current_version: str, new_version: str
) -> None:
    """Prepare pull requests with new version changes."""
    for repository in repositories.keys():
        prepare_pull_request(repository, current_version, new_version)


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    check_cli_args(sys.argv)

    current_version = sys.argv[1]
    check_version_format(current_version)
    logger.info(f"current version: {current_version}")

    new_version = sys.argv[2]
    check_version_format(new_version)
    logger.info(f"new version:     {new_version}")

    prepare_patches(repositories, current_version, new_version)

    prepare_pull_requests(repositories, current_version, new_version)
