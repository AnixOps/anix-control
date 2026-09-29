import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

class PrivacyPolicyPage extends ConsumerWidget {
  const PrivacyPolicyPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Privacy Policy'),
      ),
      body: const SingleChildScrollView(
        padding: EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'AnixOps Control Center Privacy Policy',
              style: TextStyle(fontSize: 20, fontWeight: FontWeight.bold),
            ),
            SizedBox(height: 16),
            Text(
              'Last updated: March 17, 2026',
              style: TextStyle(color: Colors.grey),
            ),
            SizedBox(height: 24),
            _SectionTitle('1. Information We Collect'),
            _SectionContent(
              'We collect information you provide directly to us, such as when you create an account, '
              'use our services, or contact us for support. This may include:\n\n'
              '• Account information (email, username)\n'
              '• Device information (model, OS version)\n'
              '• Usage data and analytics\n'
              '• Log data',
            ),
            SizedBox(height: 16),
            _SectionTitle('2. How We Use Your Information'),
            _SectionContent(
              'We use the information we collect to:\n\n'
              '• Provide, maintain, and improve our services\n'
              '• Process transactions and send related information\n'
              '• Send technical notices and support messages\n'
              '• Respond to your comments and questions\n'
              '• Monitor and analyze trends and usage',
            ),
            SizedBox(height: 16),
            _SectionTitle('3. Data Security'),
            _SectionContent(
              'We implement appropriate security measures to protect your personal information against '
              'unauthorized access, alteration, disclosure, or destruction. However, no method of '
              'transmission over the Internet is 100% secure.',
            ),
            SizedBox(height: 16),
            _SectionTitle('4. Data Retention'),
            _SectionContent(
              'We retain your personal information for as long as necessary to provide our services '
              'and comply with our legal obligations. You may request deletion of your data at any time.',
            ),
            SizedBox(height: 16),
            _SectionTitle('5. Third-Party Services'),
            _SectionContent(
              'Our app may use third-party services that have their own privacy policies:\n\n'
              '• Firebase Analytics and Crashlytics\n'
              '• Google Play Services\n\n'
              'We encourage you to review their privacy policies.',
            ),
            SizedBox(height: 16),
            _SectionTitle('6. Your Rights'),
            _SectionContent(
              'You have the right to:\n\n'
              '• Access your personal data\n'
              '• Correct inaccurate data\n'
              '• Delete your data\n'
              '• Export your data\n'
              '• Object to processing',
            ),
            SizedBox(height: 16),
            _SectionTitle('7. Children\'s Privacy'),
            _SectionContent(
              'Our services are not intended for children under 13. We do not knowingly collect '
              'personal information from children under 13.',
            ),
            SizedBox(height: 16),
            _SectionTitle('8. Changes to This Policy'),
            _SectionContent(
              'We may update this privacy policy from time to time. We will notify you of any changes '
              'by posting the new policy on this page and updating the "Last updated" date.',
            ),
            SizedBox(height: 16),
            _SectionTitle('9. Contact Us'),
            _SectionContent(
              'If you have any questions about this Privacy Policy, please contact us at:\n\n'
              'Email: privacy@anixops.com\n'
              'GitHub: https://github.com/AnixOps/anix-control',
            ),
            SizedBox(height: 32),
          ],
        ),
      ),
    );
  }
}

class _SectionTitle extends StatelessWidget {
  final String text;
  const _SectionTitle(this.text);

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: const TextStyle(
        fontSize: 16,
        fontWeight: FontWeight.bold,
      ),
    );
  }
}

class _SectionContent extends StatelessWidget {
  final String text;
  const _SectionContent(this.text);

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: const TextStyle(fontSize: 14, height: 1.5),
    );
  }
}