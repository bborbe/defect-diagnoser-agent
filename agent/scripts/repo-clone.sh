#!/usr/bin/env bash
# Clones one public GitHub repository read-only into the fixed repos/ directory.

# refuse prints the single rejection reason shared by every reference failure.
refuse() {
  echo "invalid repo reference" >&2
  exit 2
}

# usage prints the single accepted invocation shape.
usage() {
  echo "usage: repo-clone.sh clone <owner>/<name>" >&2
  exit 2
}

if [ "$#" -ne 2 ]; then
  usage
fi

if [ "$1" != "clone" ]; then
  usage
fi

ref="$2"

case "$ref" in
  https://github.com/*) slug="${ref#https://github.com/}" ;;
  git@github.com:*) slug="${ref#git@github.com:}" ;;
  *"://"*) refuse ;;
  *) slug="$ref" ;;
esac

case "$slug" in
  *.git) slug="${slug%.git}" ;;
esac

owner="${slug%%/*}"
name="${slug#*/}"

# No slash at all leaves name equal to slug; an empty half is equally unusable.
if [ "$name" = "$slug" ] || [ -z "$owner" ] || [ -z "$name" ]; then
  refuse
fi

# A name that still carries a slash means the slug had more than two segments.
case "$name" in
  */*) refuse ;;
esac

case "$owner" in
  "." | "..") refuse ;;
  *[!A-Za-z0-9._-]*) refuse ;;
esac

case "$name" in
  "." | "..") refuse ;;
  *[!A-Za-z0-9._-]*) refuse ;;
esac

# The clone root is fixed beside this script; pwd -P makes it physical so the
# containment checks below compare real paths, not symlinked ones.
script_dir="$(cd "$(dirname "$0")" && pwd -P)"
agent_dir="$(cd "$script_dir/.." && pwd -P)"
repos_dir="$agent_dir/repos"

if [ -L "$repos_dir" ]; then
  refuse
fi

# Resolve the owner component. Nothing is created here: a planted symlink must
# be seen as the path it points at, not followed by a mkdir.
if [ -e "$repos_dir/$owner" ] || [ -L "$repos_dir/$owner" ]; then
  parent="$(cd "$repos_dir/$owner" 2>/dev/null && pwd -P)"
else
  parent="$repos_dir/$owner"
fi

# An empty resolution means a dangling symlink, a symlink to a file, or a file.
if [ -z "$parent" ]; then
  refuse
fi

dest="$parent/$name"

if [ -e "$dest" ] || [ -L "$dest" ]; then
  dest_real="$(cd "$dest" 2>/dev/null && pwd -P)"
else
  dest_real="$dest"
fi

if [ -z "$dest_real" ]; then
  refuse
fi

# Defense in depth: both resolved components must stay strictly under repos/.
# The trailing slash keeps a sibling directory like repos-evil/ from matching.
case "$parent" in
  "$repos_dir"/*) ;;
  *) refuse ;;
esac

case "$dest_real" in
  "$repos_dir"/*) ;;
  *) refuse ;;
esac

# Replacing a previous read-only clone needs its write permission back first.
if [ -e "$dest" ] || [ -L "$dest" ]; then
  chmod -R u+w "$dest" 2>/dev/null
  rm -rf "$dest"
fi

mkdir -p "$parent"

url="https://github.com/${owner}/${name}"

git clone "$url" "$dest" || {
  echo "git clone failed for $url" >&2
  exit 1
}

head_sha="$(git -C "$dest" rev-parse HEAD 2>/dev/null)"
default_branch="$(git -C "$dest" rev-parse --abbrev-ref HEAD 2>/dev/null)"

if [ -e "$dest" ]; then
  chmod -R a-w "$dest"
fi

echo "clone_path=$dest"
echo "head_sha=$head_sha"
echo "default_branch=$default_branch"

exit 0
