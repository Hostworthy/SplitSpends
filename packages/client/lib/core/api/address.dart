/// True for `http://` addresses, which send sign-in codes unencrypted.
bool insecureServer(String url) =>
    url.trim().toLowerCase().startsWith('http://');

/// True for an `http://` address outside the local network, where anyone
/// on the way could read the sign-in code and session token. The app
/// checks this itself: the phones' platform network rules do not govern
/// the HTTP client it uses.
bool publicPlainServer(String url) {
  if (!insecureServer(url)) return false;
  final host = Uri.tryParse(url.trim())?.host.toLowerCase() ?? '';
  return host.isNotEmpty && !_localHost(host);
}

/// Names that never resolve on the public internet: one label (resolved by
/// the local network), localhost, mDNS (.local) and the names reserved for
/// home and private networks and for testing.
const _localSuffixes = [
  '.localhost',
  '.local',
  '.home.arpa',
  '.internal',
  '.lan',
  '.test',
];

bool _localHost(String host) {
  if (host == 'localhost' || _localSuffixes.any(host.endsWith)) return true;
  final v4 = _ipv4(host);
  if (v4 != null) {
    final [a, b, _, _] = v4;
    return a == 127 || // loopback
        a == 10 ||
        (a == 172 && b >= 16 && b <= 31) ||
        (a == 192 && b == 168) ||
        (a == 169 && b == 254); // link-local
  }
  if (host.contains(':')) {
    try {
      final v6 = Uri.parseIPv6Address(host);
      final loopback = v6.take(15).every((b) => b == 0) && v6[15] == 1;
      return loopback ||
          (v6[0] & 0xfe) == 0xfc || // unique local, fc00::/7
          (v6[0] == 0xfe && (v6[1] & 0xc0) == 0x80); // link-local, fe80::/10
    } on FormatException {
      return false;
    }
  }
  return !host.contains('.');
}

/// The four parts of a dotted IPv4 address, or null.
List<int>? _ipv4(String host) {
  final parts = host.split('.');
  if (parts.length != 4) return null;
  final bytes = [for (final p in parts) int.tryParse(p)];
  if (bytes.any((b) => b == null || b < 0 || b > 255)) return null;
  return bytes.cast<int>();
}
