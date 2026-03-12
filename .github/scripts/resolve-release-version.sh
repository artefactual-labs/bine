#!/usr/bin/env bash

set -euo pipefail

if [[ "${GITHUB_REF_NAME}" != "${DEFAULT_BRANCH}" ]]; then
  echo "Release workflow must run from ${DEFAULT_BRANCH}; got ${GITHUB_REF_NAME}." >&2
  exit 1
fi

selected=0
if [[ "${INPUT_PUBLISH_MINOR}" == "true" ]]; then
  ((selected += 1))
fi
if [[ "${INPUT_PUBLISH_PATCH}" == "true" ]]; then
  ((selected += 1))
fi
if [[ -n "${INPUT_VERSION}" ]]; then
  ((selected += 1))
fi

if [[ "${selected}" -ne 1 ]]; then
  echo "Select exactly one release mode: publish_minor, publish_patch, or version." >&2
  exit 1
fi

latest_published="$(
  gh api --paginate "repos/${GITHUB_REPOSITORY}/releases" \
    --jq '.[] | select(.draft == false and .prerelease == false) | .tag_name' \
    | sed -nE 's/^v([0-9]+\.[0-9]+\.[0-9]+)$/\1/p' \
    | sort -V \
    | tail -n 1
)"

if [[ -z "${latest_published}" ]]; then
  latest_published="0.0.0"
fi

IFS=. read -r major minor patch <<< "${latest_published}"

case "true" in
  "${INPUT_PUBLISH_MINOR}")
    target_version="v${major}.$((minor + 1)).0"
    ;;
  "${INPUT_PUBLISH_PATCH}")
    target_version="v${major}.${minor}.$((patch + 1))"
    ;;
  *)
    if [[ ! "${INPUT_VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      echo "version must match vMAJOR.MINOR.PATCH." >&2
      exit 1
    fi
    target_version="${INPUT_VERSION}"
    ;;
esac

if git rev-parse --verify --quiet "refs/tags/${target_version}" >/dev/null; then
  echo "Tag ${target_version} already exists in git." >&2
  exit 1
fi

if gh api "repos/${GITHUB_REPOSITORY}/releases/tags/${target_version}" >/dev/null 2>&1; then
  echo "Release ${target_version} already exists on GitHub." >&2
  exit 1
fi

echo "Preparing ${target_version} from ${GITHUB_SHA} (latest published: v${latest_published})."
echo "latest_published=v${latest_published}" >> "${GITHUB_OUTPUT}"
echo "version=${target_version}" >> "${GITHUB_OUTPUT}"
