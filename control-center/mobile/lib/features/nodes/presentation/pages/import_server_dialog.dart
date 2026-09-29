import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/core/theme/app_theme.dart';
import 'package:anixops_mobile/core/services/api_client.dart';

/// Server import dialog with SSH support
class ImportServerDialog extends ConsumerStatefulWidget {
  const ImportServerDialog({super.key});

  @override
  ConsumerState<ImportServerDialog> createState() => _ImportServerDialogState();
}

class _ImportServerDialogState extends ConsumerState<ImportServerDialog> {
  final _formKey = GlobalKey<FormState>();
  final _hostController = TextEditingController();
  final _portController = TextEditingController(text: '22');
  final _usernameController = TextEditingController(text: 'root');
  final _passwordController = TextEditingController();
  final _privateKeyController = TextEditingController();
  final _passphraseController = TextEditingController();
  final _nameController = TextEditingController();

  String _authType = 'password';
  bool _isLoading = false;
  bool _obscurePassword = true;
  bool _obscurePassphrase = true;
  String? _detectedType;
  bool _connectionTested = false;

  @override
  void dispose() {
    _hostController.dispose();
    _portController.dispose();
    _usernameController.dispose();
    _passwordController.dispose();
    _privateKeyController.dispose();
    _passphraseController.dispose();
    _nameController.dispose();
    super.dispose();
  }

  Future<void> _testConnection() async {
    if (!_formKey.currentState!.validate()) return;

    setState(() => _isLoading = true);

    try {
      final response = await apiClient.ssh.test(
        host: _hostController.text,
        port: int.parse(_portController.text),
        username: _usernameController.text,
        authType: _authType,
        password: _authType == 'password' ? _passwordController.text : null,
        privateKey: _authType == 'key' ? _privateKeyController.text : null,
        passphrase: _passphraseController.text.isNotEmpty ? _passphraseController.text : null,
      );

      if (response.data.success) {
        setState(() {
          _connectionTested = true;
          _detectedType = response.data.serverType ?? 'unknown';
        });

        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text('Connection successful! Server type: ${_detectedType ?? "unknown"}'),
              backgroundColor: Colors.green,
            ),
          );
        }
      } else {
        throw Exception(response.data.error ?? 'Connection failed');
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Connection failed: $e'),
            backgroundColor: Colors.red,
          ),
        );
      }
    } finally {
      setState(() => _isLoading = false);
    }
  }

  Future<void> _importServer() async {
    if (!_formKey.currentState!.validate()) return;

    setState(() => _isLoading = true);

    try {
      final response = await apiClient.ssh.import(
        host: _hostController.text,
        port: int.parse(_portController.text),
        username: _usernameController.text,
        authType: _authType,
        password: _authType == 'password' ? _passwordController.text : null,
        privateKey: _authType == 'key' ? _privateKeyController.text : null,
        passphrase: _authType == 'key' && _passphraseController.text.isNotEmpty
            ? _passphraseController.text
            : null,
        name: _nameController.text.isNotEmpty ? _nameController.text : null,
      );

      if (response.data.success) {
        if (mounted) {
          Navigator.of(context).pop(true);
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text('Server imported successfully!'),
              backgroundColor: Colors.green,
            ),
          );
        }
      } else {
        throw Exception(response.data.error ?? 'Import failed');
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Import failed: $e'),
            backgroundColor: Colors.red,
          ),
        );
      }
    } finally {
      setState(() => _isLoading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Dialog(
      child: Container(
        width: 500,
        constraints: const BoxConstraints(maxHeight: 700),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            // Header
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: AppTheme.darkSurface,
                border: Border(bottom: BorderSide(color: AppTheme.darkBorder)),
              ),
              child: Row(
                children: [
                  Icon(Icons.add_circle_outline, color: AppTheme.primaryColor),
                  const SizedBox(width: 12),
                  const Text(
                    'Import Server via SSH',
                    style: TextStyle(
                      fontSize: 18,
                      fontWeight: FontWeight.bold,
                      color: AppTheme.darkText,
                    ),
                  ),
                  const Spacer(),
                  IconButton(
                    icon: const Icon(Icons.close, color: AppTheme.darkTextSecondary),
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                ],
              ),
            ),

            // Form
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.all(16),
                child: Form(
                  key: _formKey,
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      // Connection info section
                      _buildSectionTitle('Connection'),
                      const SizedBox(height: 12),

                      Row(
                        children: [
                          Expanded(
                            flex: 3,
                            child: _buildTextField(
                              controller: _hostController,
                              label: 'Host / IP Address',
                              hint: 'e.g., 192.168.1.100 or server.example.com',
                              icon: Icons.dns_outlined,
                              validator: (v) => v?.isEmpty == true ? 'Required' : null,
                            ),
                          ),
                          const SizedBox(width: 12),
                          Expanded(
                            child: _buildTextField(
                              controller: _portController,
                              label: 'Port',
                              hint: '22',
                              icon: Icons.settings_ethernet,
                              keyboardType: TextInputType.number,
                              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                            ),
                          ),
                        ],
                      ),
                      const SizedBox(height: 12),

                      _buildTextField(
                        controller: _usernameController,
                        label: 'Username',
                        hint: 'root',
                        icon: Icons.person_outline,
                      ),
                      const SizedBox(height: 12),

                      _buildTextField(
                        controller: _nameController,
                        label: 'Server Name (Optional)',
                        hint: 'Friendly name for this server',
                        icon: Icons.label_outline,
                      ),

                      const SizedBox(height: 24),

                      // Authentication section
                      _buildSectionTitle('Authentication'),
                      const SizedBox(height: 12),

                      SegmentedButton<String>(
                        segments: const [
                          ButtonSegment(value: 'password', label: Text('Password')),
                          ButtonSegment(value: 'key', label: Text('SSH Key')),
                        ],
                        selected: {_authType},
                        onSelectionChanged: (s) => setState(() {
                          _authType = s.first;
                          _connectionTested = false;
                        }),
                      ),

                      const SizedBox(height: 16),

                      if (_authType == 'password') ...[
                        _buildTextField(
                          controller: _passwordController,
                          label: 'Password',
                          hint: 'Enter SSH password',
                          icon: Icons.lock_outline,
                          obscureText: _obscurePassword,
                          suffixIcon: IconButton(
                            icon: Icon(
                              _obscurePassword ? Icons.visibility : Icons.visibility_off,
                              color: AppTheme.darkTextSecondary,
                            ),
                            onPressed: () => setState(() => _obscurePassword = !_obscurePassword),
                          ),
                          validator: (v) => v?.isEmpty == true ? 'Required' : null,
                        ),
                      ] else ...[
                        _buildTextField(
                          controller: _privateKeyController,
                          label: 'Private Key',
                          hint: 'Paste your private key (-----BEGIN OPENSSH PRIVATE KEY-----)',
                          icon: Icons.vpn_key_outlined,
                          maxLines: 5,
                          validator: (v) => v?.isEmpty == true ? 'Required' : null,
                        ),
                        const SizedBox(height: 12),
                        _buildTextField(
                          controller: _passphraseController,
                          label: 'Passphrase (Optional)',
                          hint: 'If your key has a passphrase',
                          icon: Icons.lock_outline,
                          obscureText: _obscurePassphrase,
                          suffixIcon: IconButton(
                            icon: Icon(
                              _obscurePassphrase ? Icons.visibility : Icons.visibility_off,
                              color: AppTheme.darkTextSecondary,
                            ),
                            onPressed: () => setState(() => _obscurePassphrase = !_obscurePassphrase),
                          ),
                        ),
                      ],

                      if (_connectionTested && _detectedType != null) ...[
                        const SizedBox(height: 24),
                        Container(
                          padding: const EdgeInsets.all(12),
                          decoration: BoxDecoration(
                            color: Colors.green.withOpacity(0.1),
                            borderRadius: BorderRadius.circular(8),
                            border: Border.all(color: Colors.green.withOpacity(0.3)),
                          ),
                          child: Row(
                            children: [
                              const Icon(Icons.check_circle, color: Colors.green),
                              const SizedBox(width: 12),
                              Text(
                                'Detected: ${_detectedType!.toUpperCase()}',
                                style: const TextStyle(color: Colors.green),
                              ),
                            ],
                          ),
                        ),
                      ],
                    ],
                  ),
                ),
              ),
            ),

            // Actions
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                border: Border(top: BorderSide(color: AppTheme.darkBorder)),
              ),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.end,
                children: [
                  TextButton.icon(
                    onPressed: _isLoading ? null : _testConnection,
                    icon: const Icon(Icons.wifi_find),
                    label: const Text('Test Connection'),
                  ),
                  const SizedBox(width: 12),
                  ElevatedButton.icon(
                    onPressed: _isLoading ? null : _importServer,
                    icon: _isLoading
                        ? const SizedBox(
                            width: 16,
                            height: 16,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : const Icon(Icons.add),
                    label: const Text('Import Server'),
                    style: ElevatedButton.styleFrom(
                      backgroundColor: AppTheme.primaryColor,
                      foregroundColor: Colors.white,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildSectionTitle(String title) {
    return Text(
      title,
      style: const TextStyle(
        fontSize: 14,
        fontWeight: FontWeight.bold,
        color: AppTheme.primaryColor,
      ),
    );
  }

  Widget _buildTextField({
    required TextEditingController controller,
    required String label,
    String? hint,
    IconData? icon,
    bool obscureText = false,
    Widget? suffixIcon,
    int maxLines = 1,
    TextInputType? keyboardType,
    List<TextInputFormatter>? inputFormatters,
    String? Function(String?)? validator,
  }) {
    return TextFormField(
      controller: controller,
      obscureText: obscureText,
      maxLines: maxLines,
      keyboardType: keyboardType,
      inputFormatters: inputFormatters,
      validator: validator,
      style: const TextStyle(color: AppTheme.darkText),
      decoration: InputDecoration(
        labelText: label,
        labelStyle: const TextStyle(color: AppTheme.darkTextSecondary),
        hintText: hint,
        hintStyle: TextStyle(color: AppTheme.darkTextSecondary.withOpacity(0.5)),
        prefixIcon: icon != null ? Icon(icon, color: AppTheme.darkTextSecondary) : null,
        suffixIcon: suffixIcon,
        filled: true,
        fillColor: AppTheme.darkSurface,
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: AppTheme.darkBorder),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: AppTheme.darkBorder),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: BorderSide(color: AppTheme.primaryColor),
        ),
      ),
    );
  }
}