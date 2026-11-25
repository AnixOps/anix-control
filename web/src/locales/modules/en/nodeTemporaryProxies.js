// The node page's temporary SOCKS5 proxy section on the protocols tab: a
// proxy on an internal address that ends by itself. Same keys as
// zh-CN/nodeTemporaryProxies.js.
export default {
  nodeTemporaryProxies: {
    title: 'Temporary SOCKS5 proxy',
    description: 'A SOCKS5 proxy on an internal address of the node that closes automatically at its end time. Requires an Agent and kernel with temporary proxy support.',
    loading: 'Loading temporary proxies',
    loadFailed: 'Could not load temporary proxies',
    listen: 'Listen address',
    listenHelp: 'An internal address of the node, such as its mesh or LAN address. Public and wildcard addresses are refused.',
    port: 'Port',
    minutes: 'Duration',
    minutesUnit: 'min',
    open: 'Open',
    openFailed: 'Could not open the temporary proxy',
    passwordOnce: 'The password is shown only once.',
    address: 'Address',
    username: 'Username',
    password: 'Password',
    none: 'No open temporary proxy.',
    row: 'User {user}, closes at {date}',
    close: 'Close',
    closeFailed: 'Could not close the temporary proxy'
  }
}
