# AnixOps Contributor License Agreement

Version 1.0 · applies to contributions to `AnixOps/anix-control`

中文摘要在后面。如有分歧，以英文正文为准。

## Why This Exists

AnixOps Control is published under an open source license and also built,
together with private commercial modules, into a commercial edition
([`LICENSING.md`](LICENSING.md)). To ship your contribution in both, AnixOps
needs the right to license it on terms other than the open source license.
This agreement grants that right. You keep the copyright.

## Terms

"We" and "AnixOps" mean AnixOps (AnixOps Studio), the maintainer of this
repository. "You" means the person who signs. A "Contribution" is any code,
documentation or other material you submit to this repository, through a pull
request or otherwise, after you sign.

1. **Copyright license.** You grant us and the recipients of software we
   distribute a perpetual, worldwide, non-exclusive, royalty-free, irrevocable
   license to reproduce, modify, publicly display, publicly perform,
   sublicense and distribute your Contributions and derivative works of them,
   under any license terms, including proprietary ones.
2. **Patent license.** You grant us and those recipients a perpetual,
   worldwide, non-exclusive, royalty-free, irrevocable license under your
   patent claims that a Contribution necessarily infringes, alone or combined
   with the work it was submitted to, to make, use, sell, offer to sell,
   import and otherwise transfer the work. If anyone sues claiming the work or
   a Contribution infringes a patent, the patent licenses granted to them
   under this agreement for that work end on the date the suit is filed.
3. **You keep your rights.** You keep the copyright in your Contributions and
   may use them in any way you like. Nothing here transfers ownership.
4. **Our promise to you.** Every Contribution we merge stays available to the
   public from the public repository under the GNU Affero General Public
   License version 3 or another license approved by the Open Source
   Initiative.
5. **You may contribute.** Each Contribution is your original work, or you
   have the right to submit it under these terms. If your employer has rights
   in your work, you have its permission. You will say so in the pull request
   when a Contribution includes someone else's work, and name its source and
   license.
6. **No warranty.** Contributions are provided "as is", without warranties of
   any kind. You need not provide support for them.
7. **No obligation.** We are not required to merge or use any Contribution.

## How To Sign

In your first pull request, add one line to
[`.github/cla-signatures.txt`](.github/cla-signatures.txt):

```text
<your GitHub login> | <your name> | <YYYY-MM-DD> | CLA 1.0
```

The commit that adds the line is your signature. You sign once; later pull
requests need nothing more.

The "CLA" check (`.github/workflows/cla.yml`, `.github/scripts/check_cla.py`)
enforces three rules on every pull request:

1. Every commit author, and the pull request author, has a signature line.
   Commit with an email linked to your GitHub account, or the check cannot
   match the commit to you.
2. A pull request adds signature lines only for its own author: nobody signs
   for someone else.
3. Existing signature lines are never removed or changed.

The repository owner and bot accounts are exempt. Commits by the AI coding
agent account (`claude`) are accepted only in pull requests the owner opens:
they are the owner's work done with an AI tool. Commit authorship is an email
address in the commit and is not authenticated by git; the authenticated pull
request author is therefore always checked as well. `Co-authored-by` trailers
are not machine-checked: before merging, the reviewer confirms that every
human co-author named in a trailer has signed.

## 中文摘要

- 你保留贡献的版权，只授予 AnixOps 使用和再许可的权利，包括用于商业版。
- 同时授予必要的专利许可。
- AnixOps 承诺：合并的贡献会一直在公开仓库以 AGPL-3.0 或其他 OSI 批准的许可证提供。
- 你声明有权提交该贡献；包含他人代码时在 PR 里说明来源和许可证。
- 签署方式：第一次提 PR 时，在 `.github/cla-signatures.txt` 加一行，提交这一行即为签署。
- CI 检查：PR 作者和每个提交作者都已签署；只能为 PR 作者本人签署；已有签名不能删改。AI 编码助手账号（`claude`）的提交只在所有者本人开的 PR 中接受。
