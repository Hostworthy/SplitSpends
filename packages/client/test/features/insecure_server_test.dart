import 'package:flutter_test/flutter_test.dart';
import 'package:splitspends/features/auth/presentation/login_screen.dart';

void main() {
  test('plain HTTP server addresses get a warning', () {
    expect(insecureServer('http://192.168.1.20:8080'), isTrue);
    expect(insecureServer('  HTTP://split.example.com'), isTrue);
    expect(insecureServer('https://split.example.com'), isFalse);
    expect(insecureServer(''), isFalse);
  });

  test('plain HTTP outside the local network needs confirmation', () {
    for (final url in [
      'http://203.0.113.5:8080',
      'http://split.example.com',
      'HTTP://Split.Example.com:8080/',
      'http://172.32.0.1',
      'http://[2001:db8::1]:8080',
    ]) {
      expect(publicPlainServer(url), isTrue, reason: url);
    }
    for (final url in [
      'http://192.168.1.20:8080',
      'http://10.0.2.2:8080',
      'http://172.16.4.2',
      'http://127.0.0.1:8080',
      'http://169.254.10.1',
      'http://localhost:8080',
      'http://nas.local:8080',
      'http://nas:8080',
      'http://split.home.arpa',
      'http://[::1]:8080',
      'http://[fd12:3456::1]',
      'https://split.example.com',
      '',
    ]) {
      expect(publicPlainServer(url), isFalse, reason: url);
    }
  });
}
