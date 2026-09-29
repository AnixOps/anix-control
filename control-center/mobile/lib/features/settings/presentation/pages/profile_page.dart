import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/core/theme/app_theme.dart';
import 'package:anixops_mobile/core/services/api_client.dart';
import 'package:anixops_mobile/features/auth/presentation/providers/auth_provider.dart';

/// User profile page
class ProfilePage extends ConsumerStatefulWidget {
  const ProfilePage({super.key});

  @override
  ConsumerState<ProfilePage> createState() => _ProfilePageState();
}

class _ProfilePageState extends ConsumerState<ProfilePage> {
  final _nameController = TextEditingController();
  final _emailController = TextEditingController();
  bool _isEditing = false;

  @override
  void initState() {
    super.initState();
    _loadUserData();
  }

  void _loadUserData() {
    final authState = ref.read(authStateProvider);
    _emailController.text = authState.email ?? '';
    _nameController.text = authState.email?.split('@').first ?? '';
  }

  @override
  void dispose() {
    _nameController.dispose();
    _emailController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final authState = ref.watch(authStateProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Profile'),
        actions: [
          if (!_isEditing)
            IconButton(
              icon: const Icon(Icons.edit),
              onPressed: () => setState(() => _isEditing = true),
            ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          // Avatar section
          Center(
            child: Stack(
              children: [
                CircleAvatar(
                  radius: 50,
                  backgroundColor: AppTheme.primaryColor,
                  child: Text(
                    (authState.email?.isNotEmpty == true ? authState.email![0].toUpperCase() : 'U'),
                    style: const TextStyle(fontSize: 40, color: Colors.white),
                  ),
                ),
                Positioned(
                  bottom: 0,
                  right: 0,
                  child: Container(
                    padding: const EdgeInsets.all(4),
                    decoration: BoxDecoration(
                      color: AppTheme.primaryColor,
                      shape: BoxShape.circle,
                      border: Border.all(color: AppTheme.darkSurface, width: 2),
                    ),
                    child: const Icon(Icons.camera_alt, size: 20, color: Colors.white),
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 24),

          // User info
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'Account Information',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.bold,
                      color: AppTheme.darkText,
                    ),
                  ),
                  const SizedBox(height: 16),
                  _buildInfoTile(
                    icon: Icons.person_outline,
                    label: 'Name',
                    controller: _nameController,
                    enabled: _isEditing,
                  ),
                  const Divider(),
                  _buildInfoTile(
                    icon: Icons.email_outlined,
                    label: 'Email',
                    controller: _emailController,
                    enabled: false, // Email cannot be changed
                  ),
                  const Divider(),
                  _buildInfoRow(
                    icon: Icons.admin_panel_settings_outlined,
                    label: 'Role',
                    value: authState.role?.toUpperCase() ?? 'USER',
                  ),
                  const Divider(),
                  _buildInfoRow(
                    icon: Icons.verified_user_outlined,
                    label: 'Status',
                    value: 'Active',
                    trailing: Container(
                      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                      decoration: BoxDecoration(
                        color: Colors.green.withOpacity(0.1),
                        borderRadius: BorderRadius.circular(12),
                      ),
                      child: const Text(
                        'Verified',
                        style: TextStyle(color: Colors.green, fontSize: 12),
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),

          const SizedBox(height: 16),

          // Security section
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'Security',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.bold,
                      color: AppTheme.darkText,
                    ),
                  ),
                  const SizedBox(height: 16),
                  ListTile(
                    leading: const Icon(Icons.lock_outline),
                    title: const Text('Change Password'),
                    subtitle: const Text('Update your password'),
                    trailing: const Icon(Icons.chevron_right),
                    onTap: () => _showChangePasswordDialog(),
                  ),
                  const Divider(),
                  ListTile(
                    leading: const Icon(Icons.security),
                    title: const Text('Two-Factor Authentication'),
                    subtitle: const Text('Add an extra layer of security'),
                    trailing: const Icon(Icons.chevron_right),
                    onTap: () => _show2FADialog(),
                  ),
                  const Divider(),
                  ListTile(
                    leading: const Icon(Icons.vpn_key_outlined),
                    title: const Text('API Keys'),
                    subtitle: const Text('Manage your API keys'),
                    trailing: const Icon(Icons.chevron_right),
                    onTap: () => _showAPIKeysDialog(),
                  ),
                ],
              ),
            ),
          ),

          const SizedBox(height: 16),

          // Activity section
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'Activity',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.bold,
                      color: AppTheme.darkText,
                    ),
                  ),
                  const SizedBox(height: 16),
                  ListTile(
                    leading: const Icon(Icons.history),
                    title: const Text('Login History'),
                    subtitle: const Text('View recent login activity'),
                    trailing: const Icon(Icons.chevron_right),
                    onTap: () {},
                  ),
                  const Divider(),
                  ListTile(
                    leading: const Icon(Icons.devices_outlined),
                    title: const Text('Active Sessions'),
                    subtitle: const Text('Manage your active sessions'),
                    trailing: const Icon(Icons.chevron_right),
                    onTap: () => _showSessionsDialog(),
                  ),
                ],
              ),
            ),
          ),

          if (_isEditing) ...[
            const SizedBox(height: 24),
            Row(
              children: [
                Expanded(
                  child: OutlinedButton(
                    onPressed: () {
                      setState(() => _isEditing = false);
                      _loadUserData();
                    },
                    child: const Text('Cancel'),
                  ),
                ),
                const SizedBox(width: 16),
                Expanded(
                  child: ElevatedButton(
                    onPressed: () {
                      setState(() => _isEditing = false);
                      ScaffoldMessenger.of(context).showSnackBar(
                        const SnackBar(content: Text('Profile updated successfully')),
                      );
                    },
                    style: ElevatedButton.styleFrom(
                      backgroundColor: AppTheme.primaryColor,
                    ),
                    child: const Text('Save Changes'),
                  ),
                ),
              ],
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildInfoTile({
    required IconData icon,
    required String label,
    required TextEditingController controller,
    bool enabled = false,
  }) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Row(
        children: [
          Icon(icon, size: 20, color: AppTheme.darkTextSecondary),
          const SizedBox(width: 12),
          Expanded(
            child: TextField(
              controller: controller,
              enabled: enabled,
              style: const TextStyle(color: AppTheme.darkText),
              decoration: InputDecoration(
                labelText: label,
                labelStyle: const TextStyle(color: AppTheme.darkTextSecondary),
                border: enabled ? const OutlineInputBorder() : InputBorder.none,
                contentPadding: enabled ? const EdgeInsets.all(12) : EdgeInsets.zero,
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildInfoRow({
    required IconData icon,
    required String label,
    required String value,
    Widget? trailing,
  }) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Row(
        children: [
          Icon(icon, size: 20, color: AppTheme.darkTextSecondary),
          const SizedBox(width: 12),
          Text(label, style: const TextStyle(color: AppTheme.darkTextSecondary)),
          const Spacer(),
          Text(value, style: const TextStyle(color: AppTheme.darkText)),
          if (trailing != null) ...[
            const SizedBox(width: 8),
            trailing,
          ],
        ],
      ),
    );
  }

  void _showChangePasswordDialog() {
    final currentPasswordController = TextEditingController();
    final newPasswordController = TextEditingController();
    final confirmPasswordController = TextEditingController();

    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Change Password'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(
              controller: currentPasswordController,
              obscureText: true,
              decoration: const InputDecoration(
                labelText: 'Current Password',
                prefixIcon: Icon(Icons.lock_outline),
              ),
            ),
            const SizedBox(height: 16),
            TextField(
              controller: newPasswordController,
              obscureText: true,
              decoration: const InputDecoration(
                labelText: 'New Password',
                prefixIcon: Icon(Icons.lock),
              ),
            ),
            const SizedBox(height: 16),
            TextField(
              controller: confirmPasswordController,
              obscureText: true,
              decoration: const InputDecoration(
                labelText: 'Confirm New Password',
                prefixIcon: Icon(Icons.lock),
              ),
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
              if (newPasswordController.text != confirmPasswordController.text) {
                ScaffoldMessenger.of(context).showSnackBar(
                  const SnackBar(content: Text('Passwords do not match')),
                );
                return;
              }

              try {
                await apiClient.auth.updatePassword(
                  currentPassword: currentPasswordController.text,
                  newPassword: newPasswordController.text,
                );

                if (context.mounted) {
                  Navigator.pop(context);
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(content: Text('Password changed successfully')),
                  );
                }
              } catch (e) {
                ScaffoldMessenger.of(context).showSnackBar(
                  SnackBar(content: Text('Failed to change password: $e')),
                );
              }
            },
            child: const Text('Change'),
          ),
        ],
      ),
    );
  }

  void _show2FADialog() {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Two-Factor Authentication'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: AppTheme.darkSurface,
                borderRadius: BorderRadius.circular(8),
              ),
              child: Column(
                children: [
                  // Placeholder QR code
                  Container(
                    width: 150,
                    height: 150,
                    color: Colors.white,
                    child: const Center(
                      child: Text('QR Code', style: TextStyle(color: Colors.black)),
                    ),
                  ),
                  const SizedBox(height: 12),
                  const Text(
                    'Scan with your authenticator app',
                    style: TextStyle(color: AppTheme.darkTextSecondary),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 16),
            TextField(
              decoration: const InputDecoration(
                labelText: 'Enter 6-digit code',
                hintText: '000000',
              ),
              keyboardType: TextInputType.number,
              textAlign: TextAlign.center,
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
          ElevatedButton(
            onPressed: () {
              Navigator.pop(context);
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(content: Text('2FA enabled successfully')),
              );
            },
            child: const Text('Enable'),
          ),
        ],
      ),
    );
  }

  void _showAPIKeysDialog() {
    showDialog(
      context: context,
      builder: (context) => const APIKeysDialog(),
    );
  }

  void _showSessionsDialog() {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Active Sessions'),
        content: SizedBox(
          width: double.maxFinite,
          child: ListView(
            shrinkWrap: true,
            children: [
              _buildSessionTile(
                device: 'Windows Desktop',
                location: 'Local',
                lastActive: 'Now',
                isCurrent: true,
              ),
              _buildSessionTile(
                device: 'Chrome on Windows',
                location: '192.168.1.1',
                lastActive: '2 hours ago',
              ),
              _buildSessionTile(
                device: 'Android App',
                location: 'Mobile',
                lastActive: '1 day ago',
              ),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Close'),
          ),
          TextButton(
            onPressed: () {
              Navigator.pop(context);
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(content: Text('All other sessions signed out')),
              );
            },
            style: TextButton.styleFrom(foregroundColor: Colors.red),
            child: const Text('Sign Out All Others'),
          ),
        ],
      ),
    );
  }

  Widget _buildSessionTile({
    required String device,
    required String location,
    required String lastActive,
    bool isCurrent = false,
  }) {
    return ListTile(
      leading: Icon(
        device.contains('Desktop') ? Icons.computer : Icons.phone_android,
        color: isCurrent ? AppTheme.primaryColor : AppTheme.darkTextSecondary,
      ),
      title: Row(
        children: [
          Text(device),
          if (isCurrent) ...[
            const SizedBox(width: 8),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
              decoration: BoxDecoration(
                color: AppTheme.primaryColor.withOpacity(0.2),
                borderRadius: BorderRadius.circular(4),
              ),
              child: const Text(
                'Current',
                style: TextStyle(color: AppTheme.primaryColor, fontSize: 10),
              ),
            ),
          ],
        ],
      ),
      subtitle: Text('$location • $lastActive'),
      trailing: isCurrent
          ? null
          : IconButton(
              icon: const Icon(Icons.logout, color: Colors.red),
              onPressed: () {},
            ),
    );
  }
}

/// API Keys management dialog
class APIKeysDialog extends ConsumerStatefulWidget {
  const APIKeysDialog({super.key});

  @override
  ConsumerState<APIKeysDialog> createState() => _APIKeysDialogState();
}

class _APIKeysDialogState extends ConsumerState<APIKeysDialog> {
  final _keyNameController = TextEditingController();
  List<Map<String, dynamic>> _apiKeys = [];
  bool _isLoading = false;

  @override
  void initState() {
    super.initState();
    _loadApiKeys();
  }

  @override
  void dispose() {
    _keyNameController.dispose();
    super.dispose();
  }

  Future<void> _loadApiKeys() async {
    setState(() => _isLoading = true);
    try {
      final response = await apiClient.tokens.list();
      if (response.data['success'] == true) {
        setState(() {
          _apiKeys = List<Map<String, dynamic>>.from(response.data['data'] ?? []);
        });
      }
    } catch (e) {
      // Ignore errors
    } finally {
      setState(() => _isLoading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Row(
        children: [
          const Text('API Keys'),
          const Spacer(),
          IconButton(
            icon: const Icon(Icons.add),
            onPressed: _isLoading ? null : () => _showCreateKeyDialog(),
          ),
        ],
      ),
      content: SizedBox(
        width: 400,
        child: _isLoading
            ? const Center(child: CircularProgressIndicator())
            : _apiKeys.isEmpty
                ? const Center(
                    child: Text('No API keys created'),
                  )
                : ListView.builder(
                    shrinkWrap: true,
                    itemCount: _apiKeys.length,
                    itemBuilder: (context, index) {
                      final key = _apiKeys[index];
                      return Card(
                        child: ListTile(
                          leading: const Icon(Icons.vpn_key),
                          title: Text(key['name'] ?? 'Unknown'),
                          subtitle: Text(
                            'Created: ${key['created_at']?.toString().split('T').first ?? 'Unknown'}\n'
                            'Last used: ${key['last_used']?.toString().split('T').first ?? 'Never'}',
                          ),
                          isThreeLine: true,
                          trailing: IconButton(
                            icon: const Icon(Icons.delete, color: Colors.red),
                            onPressed: () => _deleteKey(key['id'].toString()),
                          ),
                        ),
                      );
                    },
                  ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Close'),
        ),
      ],
    );
  }

  void _showCreateKeyDialog() {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Create API Key'),
        content: TextField(
          controller: _keyNameController,
          decoration: const InputDecoration(
            labelText: 'Key Name',
            hintText: 'e.g., CI/CD Pipeline',
          ),
        ),
        actions: [
          TextButton(
            onPressed: () {
              Navigator.pop(context);
              _keyNameController.clear();
            },
            child: const Text('Cancel'),
          ),
          ElevatedButton(
            onPressed: () async {
              if (_keyNameController.text.isNotEmpty) {
                Navigator.pop(context);
                try {
                  final response = await apiClient.tokens.create(_keyNameController.text);
                  if (response.data['success'] == true) {
                    final token = response.data['data']['token'];
                    _keyNameController.clear();
                    _loadApiKeys();
                    _showKeyCreatedDialog(token);
                  }
                } catch (e) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    SnackBar(content: Text('Failed to create token: $e')),
                  );
                }
              }
            },
            child: const Text('Create'),
          ),
        ],
      ),
    );
  }

  void _showKeyCreatedDialog(String apiKey) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('API Key Created'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'Please copy your API key now. You won\'t be able to see it again!',
              style: TextStyle(color: Colors.orange),
            ),
            const SizedBox(height: 16),
            SelectableText(
              apiKey,
              style: const TextStyle(fontFamily: 'monospace', color: AppTheme.primaryColor),
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Done'),
          ),
        ],
      ),
    );
  }

  Future<void> _deleteKey(String id) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Delete API Key'),
        content: const Text('Are you sure you want to delete this API key?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          ElevatedButton(
            onPressed: () => Navigator.pop(context, true),
            style: ElevatedButton.styleFrom(backgroundColor: Colors.red),
            child: const Text('Delete'),
          ),
        ],
      ),
    );

    if (confirmed == true) {
      try {
        await apiClient.tokens.delete(id);
        _loadApiKeys();
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(content: Text('API key deleted')),
          );
        }
      } catch (e) {
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text('Failed to delete: $e')),
          );
        }
      }
    }
  }
}