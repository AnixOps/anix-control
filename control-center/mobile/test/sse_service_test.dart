import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:anixops_mobile/core/services/sse_service.dart';

void main() {
  group('SSEService', () {
    test('dispatches Workers event payload by type field', () {
      final service = SSEService();
      dynamic received;

      service.on('node_update', (payload) {
        received = payload;
      });

      service.parseTestEvent(
        'data: ${jsonEncode({'type': 'node_update', 'payload': {'node_id': 1, 'status': 'online'}})}',
      );

      expect(received, {'node_id': 1, 'status': 'online'});
      service.dispose();
    });

    test('subscribe and unsubscribe hit full /api/v1/sse endpoints', () async {
      final requests = <Uri>[];
      final mockClient = MockClient((request) async {
        requests.add(request.url);
        return http.Response('{"success":true}', 200);
      });

      final service = SSEService(clientFactory: () => mockClient);
      await service.connect('https://api.anixops.com/api/v1/sse', token: 'token');

      final subscribed = await service.subscribe('nodes');
      final unsubscribed = await service.unsubscribe('nodes');

      expect(subscribed, isTrue);
      expect(unsubscribed, isTrue);
      expect(requests[0].toString(), 'https://api.anixops.com/api/v1/sse');
      expect(requests[1].toString(), 'https://api.anixops.com/api/v1/sse/subscribe');
      expect(requests[2].toString(), 'https://api.anixops.com/api/v1/sse/unsubscribe');

      service.dispose();
    });
  });
}
