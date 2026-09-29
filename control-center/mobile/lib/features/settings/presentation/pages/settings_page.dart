import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'privacy_policy_page.dart';
import 'terms_of_service_page.dart';
import 'profile_page.dart';
import '../../../auth/presentation/providers/auth_provider.dart';
import '../../../../core/providers/locale_provider.dart';
import '../providers/mfa_provider.dart';

// Settings State
class SettingsState {
  final bool twoFactorEnabled;
  final bool autoBackup;
  final String theme;
  final String language;
  final bool notificationsEnabled;
  final int sessionTimeout;

  const SettingsState({
    this.twoFactorEnabled = false,
    this.autoBackup = true,
    this.theme = 'dark',
    this.language = 'en',
    this.notificationsEnabled = true,
    this.sessionTimeout = 30,
  });

  SettingsState copyWith({
    bool? twoFactorEnabled,
    bool? autoBackup,
    String? theme,
    String? language,
    bool? notificationsEnabled,
    int? sessionTimeout,
  }) {
    return SettingsState(
      twoFactorEnabled: twoFactorEnabled ?? this.twoFactorEnabled,
      autoBackup: autoBackup ?? this.autoBackup,
      theme: theme ?? this.theme,
      language: language ?? this.language,
      notificationsEnabled: notificationsEnabled ?? this.notificationsEnabled,
      sessionTimeout: sessionTimeout ?? this.sessionTimeout,
    );
  }
}

class SettingsNotifier extends Notifier<SettingsState> {
  @override
  SettingsState build() => const SettingsState();

  void setTwoFactor(bool value) => state = state.copyWith(twoFactorEnabled: value);
  void setAutoBackup(bool value) => state = state.copyWith(autoBackup: value);
  void setTheme(String value) => state = state.copyWith(theme: value);
  void setLanguage(String value) => state = state.copyWith(language: value);
  void setNotifications(bool value) => state = state.copyWith(notificationsEnabled: value);
  void setSessionTimeout(int value) => state = state.copyWith(sessionTimeout: value);
}

final settingsProvider = NotifierProvider<SettingsNotifier, SettingsState>(SettingsNotifier.new);

// Settings Page
class SettingsPage extends ConsumerWidget {
  const SettingsPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(settingsProvider);
    final theme = Theme.of(context);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Settings'),
      ),
      body: ListView(
        children: [
          // Account Section
          _buildSectionHeader('Account', context),
          Builder(
            builder: (context) {
              final authState = ref.watch(authStateProvider);
              final userEmail = authState.email ?? 'user@example.com';
              final userName = userEmail.split('@').first;

              return ListTile(
                leading: CircleAvatar(
                  backgroundColor: theme.colorScheme.primary,
                  child: Text(
                    userName.isNotEmpty ? userName[0].toUpperCase() : 'U',
                    style: const TextStyle(color: Colors.white, fontWeight: FontWeight.bold),
                  ),
                ),
                title: Text(userName),
                subtitle: Text(userEmail),
                trailing: const Icon(Icons.chevron_right),
                onTap: () {
                  Navigator.push(
                    context,
                    MaterialPageRoute(builder: (context) => const ProfilePage()),
                  );
                },
              );
            },
          ),
          ListTile(
            leading: const Icon(Icons.lock_outline),
            title: const Text('Change Password'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              Navigator.push(
                context,
                MaterialPageRoute(builder: (context) => const ProfilePage()),
              );
            },
          ),
          SwitchListTile(
            secondary: const Icon(Icons.security),
            title: const Text('Two-Factor Authentication'),
            subtitle: Text(
              ref.watch(mfaProvider).status?.enabled == true
                  ? 'Enabled - Tap to manage'
                  : 'Add extra security to your account',
            ),
            value: ref.watch(mfaProvider).status?.enabled ?? false,
            onChanged: (value) {
              if (value) {
                _showMFASetupDialog(context, ref);
              } else {
                _showMFADisableDialog(context, ref);
              }
            },
          ),
          ListTile(
            leading: const Icon(Icons.vpn_key_outlined),
            title: const Text('API Keys'),
            subtitle: const Text('Manage API access tokens'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              showDialog(
                context: context,
                builder: (context) => const APIKeysDialog(),
              );
            },
          ),

          const Divider(),

          // Server Section
          _buildSectionHeader('Server Configuration', context),
          ListTile(
            leading: const Icon(Icons.dns_outlined),
            title: const Text('Server Settings'),
            subtitle: const Text('Host, port, SSL configuration'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              // Navigate to server settings
            },
          ),
          ListTile(
            leading: const Icon(Icons.storage_outlined),
            title: const Text('Database'),
            subtitle: const Text('SQLite'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              // Navigate to database settings
            },
          ),

          const Divider(),

          // Plugins Section
          _buildSectionHeader('Plugins', context),
          ListTile(
            leading: const Icon(Icons.extension_outlined),
            title: const Text('Manage Plugins'),
            subtitle: const Text('4 plugins installed'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              // Navigate to plugins
            },
          ),

          const Divider(),

          // Security Section
          _buildSectionHeader('Security', context),
          ListTile(
            leading: const Icon(Icons.admin_panel_settings_outlined),
            title: const Text('IP Whitelist'),
            subtitle: const Text('Restrict access by IP'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              // Navigate to IP whitelist
            },
          ),
          ListTile(
            leading: const Icon(Icons.timer_outlined),
            title: const Text('Session Timeout'),
            subtitle: Text('${state.sessionTimeout} minutes'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              _showTimeoutDialog(context, ref, state.sessionTimeout);
            },
          ),

          const Divider(),

          // Notifications Section
          _buildSectionHeader('Notifications', context),
          SwitchListTile(
            secondary: const Icon(Icons.notifications_outlined),
            title: const Text('Push Notifications'),
            subtitle: const Text('Receive alerts on your device'),
            value: state.notificationsEnabled,
            onChanged: (value) => ref.read(settingsProvider.notifier).setNotifications(value),
          ),
          ListTile(
            leading: const Icon(Icons.email_outlined),
            title: const Text('Email Settings'),
            subtitle: const Text('Configure email notifications'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              // Navigate to email settings
            },
          ),

          const Divider(),

          // Backup Section
          _buildSectionHeader('Backup & Restore', context),
          SwitchListTile(
            secondary: const Icon(Icons.backup_outlined),
            title: const Text('Automatic Backup'),
            subtitle: const Text('Daily at midnight'),
            value: state.autoBackup,
            onChanged: (value) => ref.read(settingsProvider.notifier).setAutoBackup(value),
          ),
          ListTile(
            leading: const Icon(Icons.cloud_download_outlined),
            title: const Text('Create Backup'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              _showBackupDialog(context);
            },
          ),
          ListTile(
            leading: const Icon(Icons.restore_outlined),
            title: const Text('Restore from Backup'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              // Show restore dialog
            },
          ),

          const Divider(),

          // Appearance Section
          _buildSectionHeader('Appearance', context),
          ListTile(
            leading: const Icon(Icons.palette_outlined),
            title: const Text('Theme'),
            subtitle: Text(state.theme == 'dark' ? 'Dark' : 'Light'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              _showThemeDialog(context, ref, state.theme);
            },
          ),
          ListTile(
            leading: const Icon(Icons.language),
            title: const Text('Language'),
            subtitle: Text(state.language == 'en' ? 'English' : '中文'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              _showLanguageDialog(context, ref, state.language);
            },
          ),

          const Divider(),

          // About Section
          _buildSectionHeader('About', context),
          ListTile(
            leading: const Icon(Icons.info_outline),
            title: const Text('Version'),
            subtitle: const Text('0.9.9'),
          ),
          ListTile(
            leading: const Icon(Icons.update),
            title: const Text('Check for Updates'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(content: Text('You are running the latest version')),
              );
            },
          ),
          ListTile(
            leading: const Icon(Icons.description_outlined),
            title: const Text('Documentation'),
            trailing: const Icon(Icons.open_in_new),
            onTap: () {
              // Open documentation
            },
          ),
          ListTile(
            leading: const Icon(Icons.bug_report_outlined),
            title: const Text('Report an Issue'),
            trailing: const Icon(Icons.open_in_new),
            onTap: () {
              // Open issue tracker
            },
          ),
          ListTile(
            leading: const Icon(Icons.privacy_tip_outlined),
            title: const Text('Privacy Policy'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              Navigator.push(
                context,
                MaterialPageRoute(builder: (context) => const PrivacyPolicyPage()),
              );
            },
          ),
          ListTile(
            leading: const Icon(Icons.gavel_outlined),
            title: const Text('Terms of Service'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () {
              Navigator.push(
                context,
                MaterialPageRoute(builder: (context) => const TermsOfServicePage()),
              );
            },
          ),

          const SizedBox(height: 24),

          // Logout Button
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: OutlinedButton.icon(
              onPressed: () {
                _showLogoutDialog(context, ref);
              },
              icon: const Icon(Icons.logout),
              label: const Text('Sign Out'),
              style: OutlinedButton.styleFrom(
                foregroundColor: Colors.red,
                side: const BorderSide(color: Colors.red),
                padding: const EdgeInsets.symmetric(vertical: 12),
              ),
            ),
          ),

          const SizedBox(height: 32),
        ],
      ),
    );
  }

  Widget _buildSectionHeader(String title, BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
      child: Text(
        title,
        style: Theme.of(context).textTheme.titleSmall?.copyWith(
          color: Theme.of(context).colorScheme.primary,
          fontWeight: FontWeight.bold,
        ),
      ),
    );
  }

  void _showTimeoutDialog(BuildContext context, WidgetRef ref, int currentValue) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Session Timeout'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            RadioListTile<int>(
              title: const Text('15 minutes'),
              value: 15,
              groupValue: currentValue,
              onChanged: (value) {
                ref.read(settingsProvider.notifier).setSessionTimeout(value!);
                Navigator.pop(context);
              },
            ),
            RadioListTile<int>(
              title: const Text('30 minutes'),
              value: 30,
              groupValue: currentValue,
              onChanged: (value) {
                ref.read(settingsProvider.notifier).setSessionTimeout(value!);
                Navigator.pop(context);
              },
            ),
            RadioListTile<int>(
              title: const Text('1 hour'),
              value: 60,
              groupValue: currentValue,
              onChanged: (value) {
                ref.read(settingsProvider.notifier).setSessionTimeout(value!);
                Navigator.pop(context);
              },
            ),
            RadioListTile<int>(
              title: const Text('Never'),
              value: 0,
              groupValue: currentValue,
              onChanged: (value) {
                ref.read(settingsProvider.notifier).setSessionTimeout(value!);
                Navigator.pop(context);
              },
            ),
          ],
        ),
      ),
    );
  }

  void _showThemeDialog(BuildContext context, WidgetRef ref, String currentValue) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Theme'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            RadioListTile<String>(
              title: const Text('Light'),
              value: 'light',
              groupValue: currentValue,
              onChanged: (value) {
                ref.read(settingsProvider.notifier).setTheme(value!);
                ref.read(themeModeProvider.notifier).setThemeMode(ThemeMode.light);
                Navigator.pop(context);
              },
            ),
            RadioListTile<String>(
              title: const Text('Dark'),
              value: 'dark',
              groupValue: currentValue,
              onChanged: (value) {
                ref.read(settingsProvider.notifier).setTheme(value!);
                ref.read(themeModeProvider.notifier).setThemeMode(ThemeMode.dark);
                Navigator.pop(context);
              },
            ),
            RadioListTile<String>(
              title: const Text('System'),
              value: 'system',
              groupValue: currentValue,
              onChanged: (value) {
                ref.read(settingsProvider.notifier).setTheme(value!);
                ref.read(themeModeProvider.notifier).setThemeMode(ThemeMode.system);
                Navigator.pop(context);
              },
            ),
          ],
        ),
      ),
    );
  }

  void _showLanguageDialog(BuildContext context, WidgetRef ref, String currentValue) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Language'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            RadioListTile<String>(
              title: const Text('English'),
              value: 'en',
              groupValue: currentValue,
              onChanged: (value) {
                ref.read(settingsProvider.notifier).setLanguage(value!);
                ref.read(localeProvider.notifier).setLocale(const Locale('en'));
                Navigator.pop(context);
              },
            ),
            RadioListTile<String>(
              title: const Text('简体中文'),
              value: 'zh',
              groupValue: currentValue,
              onChanged: (value) {
                ref.read(settingsProvider.notifier).setLanguage(value!);
                ref.read(localeProvider.notifier).setLocale(const Locale('zh'));
                Navigator.pop(context);
              },
            ),
            RadioListTile<String>(
              title: const Text('日本語'),
              value: 'ja',
              groupValue: currentValue,
              onChanged: (value) {
                ref.read(settingsProvider.notifier).setLanguage(value!);
                ref.read(localeProvider.notifier).setLocale(const Locale('ja', 'JP'));
                Navigator.pop(context);
              },
            ),
            RadioListTile<String>(
              title: const Text('繁體中文'),
              value: 'zh_TW',
              groupValue: currentValue,
              onChanged: (value) {
                ref.read(settingsProvider.notifier).setLanguage(value!);
                ref.read(localeProvider.notifier).setLocale(const Locale('zh', 'TW'));
                Navigator.pop(context);
              },
            ),
            RadioListTile<String>(
              title: const Text('العربية'),
              value: 'ar',
              groupValue: currentValue,
              onChanged: (value) {
                ref.read(settingsProvider.notifier).setLanguage(value!);
                ref.read(localeProvider.notifier).setLocale(const Locale('ar', 'SA'));
                Navigator.pop(context);
              },
            ),
          ],
        ),
      ),
    );
  }

  void _showBackupDialog(BuildContext context) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Create Backup'),
        content: const Text('This will create a backup of your database and configuration. Continue?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
          ElevatedButton(
            onPressed: () {
              Navigator.pop(context);
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(content: Text('Backup created successfully')),
              );
            },
            child: const Text('Create'),
          ),
        ],
      ),
    );
  }

  void _showLogoutDialog(BuildContext context, WidgetRef ref) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Sign Out'),
        content: const Text('Are you sure you want to sign out?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
          ElevatedButton(
            onPressed: () async {
              Navigator.pop(context);
              // Perform actual logout
              await ref.read(authStateProvider.notifier).logout();
              // Navigate to login
              if (context.mounted) {
                context.go('/login');
              }
            },
            style: ElevatedButton.styleFrom(
              backgroundColor: Colors.red,
            ),
            child: const Text('Sign Out'),
          ),
        ],
      ),
    );
  }

  void _showMFASetupDialog(BuildContext context, WidgetRef ref) {
    showDialog(
      context: context,
      barrierDismissible: false,
      builder: (context) => _MFASetupDialog(ref: ref),
    );
  }

  void _showMFADisableDialog(BuildContext context, WidgetRef ref) {
    final codeController = TextEditingController();

    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Disable Two-Factor Authentication'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Text('Enter your verification code to disable 2FA:'),
            const SizedBox(height: 16),
            TextField(
              controller: codeController,
              decoration: const InputDecoration(
                labelText: 'Verification Code',
                hintText: '6-digit code',
              ),
              keyboardType: TextInputType.number,
              maxLength: 6,
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
          ElevatedButton(
            onPressed: () async {
              Navigator.pop(context);
              final success = await ref.read(mfaProvider.notifier).disable(codeController.text);
              if (context.mounted) {
                ScaffoldMessenger.of(context).showSnackBar(
                  SnackBar(content: Text(success ? '2FA disabled' : 'Failed to disable 2FA')),
                );
              }
            },
            style: ElevatedButton.styleFrom(backgroundColor: Colors.red),
            child: const Text('Disable'),
          ),
        ],
      ),
    );
  }
}

/// MFA Setup Dialog with QR code display
class _MFASetupDialog extends ConsumerStatefulWidget {
  final WidgetRef ref;

  const _MFASetupDialog({required this.ref});

  @override
  ConsumerState<_MFASetupDialog> createState() => _MFASetupDialogState();
}

class _MFASetupDialogState extends ConsumerState<_MFASetupDialog> {
  final _codeController = TextEditingController();
  bool _isVerifying = false;

  @override
  void initState() {
    super.initState();
    Future.microtask(() => ref.read(mfaProvider.notifier).setup());
  }

  @override
  void dispose() {
    _codeController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final mfaState = ref.watch(mfaProvider);

    return AlertDialog(
      title: const Text('Setup Two-Factor Authentication'),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (mfaState.isLoading)
              const Center(child: CircularProgressIndicator())
            else if (mfaState.setupResult != null) ...[
              const Text('1. Scan this QR code with your authenticator app:'),
              const SizedBox(height: 16),
              // QR code placeholder - in real app, use qr_flutter package
              Container(
                width: 200,
                height: 200,
                decoration: BoxDecoration(
                  border: Border.all(color: Colors.grey),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    const Icon(Icons.qr_code_2, size: 100),
                    Text(
                      'Secret: ${mfaState.setupResult!.secret.substring(0, 8)}...',
                      style: const TextStyle(fontSize: 10),
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 16),
              const Text('2. Enter the 6-digit code from your app:'),
              const SizedBox(height: 8),
              TextField(
                controller: _codeController,
                decoration: const InputDecoration(
                  labelText: 'Verification Code',
                  hintText: '000000',
                ),
                keyboardType: TextInputType.number,
                maxLength: 6,
              ),
              const SizedBox(height: 16),
              const Text('3. Save these recovery codes:'),
              const SizedBox(height: 8),
              Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  color: Colors.grey.withValues(alpha: 0.1),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Column(
                  children: mfaState.setupResult!.recoveryCodes.take(4).map((code) {
                    return Text(code, style: const TextStyle(fontFamily: 'monospace'));
                  }).toList(),
                ),
              ),
            ],
            if (mfaState.error != null)
              Text(
                mfaState.error!,
                style: const TextStyle(color: Colors.red),
              ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () {
            ref.read(mfaProvider.notifier).clearSetupResult();
            Navigator.pop(context);
          },
          child: const Text('Cancel'),
        ),
        if (mfaState.setupResult != null)
          ElevatedButton(
            onPressed: _isVerifying
                ? null
                : () async {
                    setState(() => _isVerifying = true);
                    final success = await ref.read(mfaProvider.notifier).enable(_codeController.text);
                    setState(() => _isVerifying = false);

                    if (success && context.mounted) {
                      Navigator.pop(context);
                      ScaffoldMessenger.of(context).showSnackBar(
                        const SnackBar(content: Text('2FA enabled successfully')),
                      );
                    }
                  },
            child: _isVerifying
                ? const SizedBox(
                    width: 16,
                    height: 16,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Text('Verify & Enable'),
          ),
      ],
    );
  }
}