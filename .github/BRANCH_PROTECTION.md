# 分支保护规则配置
#
# 此文件描述了 production 分支的保护规则
# 需要在 GitHub 仓库设置中手动配置
#
# ==========================================
# GitHub 仓库设置路径:
# Settings -> Branches -> Add branch protection rule
# ==========================================

# Production 分支保护规则:

## 1. 基本保护
- [x] Require a pull request before merging
  - Require approvals: 1
  - Dismiss stale pull request approvals when new commits are pushed
  - Require review from Code Owners

## 2. 状态检查
- [x] Require status checks to pass before merging
  - Status checks that are required:
    - Test & Coverage
    - gRPC Core Tests
    - Build

## 3. 分支限制
- [x] Restrict who can push to matching branches
  - Only allow specified users/teams

## 4. 代码所有者
# 创建 .github/CODEOWNERS 文件

## 5. 签名要求
- [x] Require signed commits

## 6. 线性历史
- [x] Require linear history

## 7. 强制推送
- [x] Do not allow force pushes

## 8. 删除保护
- [x] Do not allow deletions

# ==========================================
# 使用 gh CLI 配置分支保护 (需要管理员权限):
# ==========================================

# gh api repos/:owner/:repo/branches/production/protection \
#   --method PUT \
#   --field required_status_checks='{"strict":true,"contexts":["Test & Coverage","gRPC Core Tests","Build"]}' \
#   --field enforce_admins=true \
#   --field required_pull_request_reviews='{"dismiss_stale_reviews":true,"require_code_owner_reviews":true,"required_approving_review_count":1}' \
#   --field restrictions=null \
#   --field required_linear_history=true \
#   --field allow_force_pushes=false \
#   --field allow_deletions=false