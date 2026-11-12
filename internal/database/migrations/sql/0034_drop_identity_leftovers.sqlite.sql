-- 0034_drop_identity_leftovers (SQLite): identity holds logins and second
-- factors, and the cutover ran on 5.0.3. The kernel's MFA login attempts
-- (v2_mfa_login_attempt) and the cutover log (v4_kernel_identity_cutover)
-- have no writer or reader left; they are dropped with their models.
DROP TABLE IF EXISTS `v2_mfa_login_attempt`;
DROP TABLE IF EXISTS `v4_kernel_identity_cutover`;
