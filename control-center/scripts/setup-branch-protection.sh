#!/bin/bash

# Branch Protection Setup Script
# Usage: ./scripts/setup-branch-protection.sh <github-token> <owner/repo>

set -e

TOKEN="${1:-$GITHUB_TOKEN}"
REPO="${2:-$(git remote get-url origin | sed 's/.*github.com[/:]//' | sed 's/.git$//')}"

if [ -z "$TOKEN" ]; then
    echo "Error: GitHub token required"
    echo "Usage: $0 <token> [owner/repo]"
    exit 1
fi

API_URL="https://api.github.com/repos/${REPO}"

echo "Setting up branch protection for ${REPO}..."

# Function to set branch protection
set_protection() {
    local branch=$1
    local required_reviews=$2
    local required_contexts=$3
    local enforce_admins=$4

    echo "Configuring branch: ${branch}"

    curl -s -X PUT \
        -H "Authorization: token ${TOKEN}" \
        -H "Accept: application/vnd.github.v3+json" \
        "${API_URL}/branches/${branch}/protection" \
        -d "$(cat <<EOF
{
    "required_status_checks": {
        "strict": true,
        "contexts": ${required_contexts}
    },
    "enforce_admins": ${enforce_admins},
    "required_pull_request_reviews": {
        "dismiss_stale_reviews": true,
        "require_code_owner_reviews": true,
        "required_approving_review_count": ${required_reviews}
    },
    "restrictions": null,
    "required_linear_history": true,
    "allow_force_pushes": false,
    "allow_deletions": false,
    "required_conversation_resolution": true
}
EOF
)" || echo "Warning: Failed to set protection for ${branch}"
}

# Production branch - Strict protection
set_protection "production" 2 \
    '["Lint", "Test", "Critical Tests", "Production Quality Gate (80% + Critical 100%)", "Build"]' \
    true

# Dev branch - Moderate protection
set_protection "dev" 1 \
    '["Lint", "Test", "Dev Quality Gate (60%)", "Build"]' \
    false

echo "Branch protection setup complete!"
echo ""
echo "Note: Version branches (v*.*.*) should be protected manually via GitHub UI"
echo "      to prevent any commits after version freeze."