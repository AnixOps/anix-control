export default {
  networkPages: {
    nodes: {
      table: {
        id: 'ID'
      },
      protocols: {
        vmess: 'VMess',
        vless: 'VLESS',
        trojan: 'Trojan',
        shadowsocks: 'Shadowsocks',
        hysteria2: 'Hysteria2',
        tuic: 'TUIC'
      },
      protocolPlaceholders: {
        settings: '{"flow":"xtls-rprx-vision"}',
        tlsSettings: '{"server_name":"example.com"}',
        realitySettings: '{"short_id":"..."}',
        transportSettings: '{"path":"/ws"}',
        customConfig: '{"node_type":"vless", ...}'
      }
    },
    subscriptions: {
      nodeIdFallback: 'ID: {id}',
      protocols: {
        vmess: 'VMess',
        vless: 'VLESS',
        trojan: 'Trojan',
        shadowsocks: 'Shadowsocks',
        hysteria2: 'Hysteria2',
        tuic: 'TUIC'
      },
      tlsModes: {
        tls: 'TLS',
        reality: 'Reality'
      },
      transports: {
        tcp: 'TCP',
        ws: 'WebSocket',
        grpc: 'gRPC',
        h2: 'HTTP/2',
        quic: 'QUIC'
      },
      fingerprints: {
        chrome: 'Chrome',
        firefox: 'Firefox',
        safari: 'Safari',
        edge: 'Edge',
        random: 'Random'
      },
      flows: {
        xtlsRprxVision: 'xtls-rprx-vision'
      },
      placeholders: {
        server: 'us.example.com',
        port: '443',
        sni: 'www.example.com',
        wsPath: '/ws'
      }
    }
  }
}
