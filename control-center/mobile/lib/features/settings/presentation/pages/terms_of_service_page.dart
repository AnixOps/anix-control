import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

class TermsOfServicePage extends ConsumerWidget {
  const TermsOfServicePage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Terms of Service'),
      ),
      body: const SingleChildScrollView(
        padding: EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'AnixOps Control Center Terms of Service',
              style: TextStyle(fontSize: 20, fontWeight: FontWeight.bold),
            ),
            SizedBox(height: 16),
            Text(
              'Last updated: March 17, 2026',
              style: TextStyle(color: Colors.grey),
            ),
            SizedBox(height: 24),
            _SectionTitle('1. Acceptance of Terms'),
            _SectionContent(
              'By accessing and using AnixOps Control Center, you accept and agree to be bound by '
              'these Terms of Service. If you do not agree to these terms, please do not use our services.',
            ),
            SizedBox(height: 16),
            _SectionTitle('2. Description of Service'),
            _SectionContent(
              'AnixOps Control Center is an infrastructure management platform that provides tools '
              'for managing servers, nodes, and related services. The service includes mobile, web, '
              'and desktop applications.',
            ),
            SizedBox(height: 16),
            _SectionTitle('3. User Accounts'),
            _SectionContent(
              'You are responsible for maintaining the security of your account and password. '
              'You agree to accept responsibility for all activities that occur under your account.',
            ),
            SizedBox(height: 16),
            _SectionTitle('4. Acceptable Use'),
            _SectionContent(
              'You agree not to use the service:\n\n'
              '• For any unlawful purpose\n'
              '• To violate any laws or regulations\n'
              '• To infringe on the rights of others\n'
              '• To distribute malware or harmful code\n'
              '• To interfere with the service operations',
            ),
            SizedBox(height: 16),
            _SectionTitle('5. Intellectual Property'),
            _SectionContent(
              'The service and its original content are owned by AnixOps and are protected by '
              'international copyright, trademark, and other intellectual property laws.',
            ),
            SizedBox(height: 16),
            _SectionTitle('6. Disclaimer of Warranties'),
            _SectionContent(
              'The service is provided "as is" without warranties of any kind, either express or implied. '
              'We do not guarantee that the service will be uninterrupted or error-free.',
            ),
            SizedBox(height: 16),
            _SectionTitle('7. Limitation of Liability'),
            _SectionContent(
              'In no event shall AnixOps be liable for any indirect, incidental, special, consequential, '
              'or punitive damages arising out of your use of the service.',
            ),
            SizedBox(height: 16),
            _SectionTitle('8. Changes to Terms'),
            _SectionContent(
              'We reserve the right to modify these terms at any time. We will notify users of any '
              'material changes by posting the new terms on this page.',
            ),
            SizedBox(height: 16),
            _SectionTitle('9. Contact'),
            _SectionContent(
              'For questions about these Terms of Service, please contact:\n\n'
              'Email: legal@anixops.com\n'
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