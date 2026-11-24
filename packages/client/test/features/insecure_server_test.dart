import 'package:flutter_test/flutter_test.dart';
import 'package:splitspends/features/auth/presentation/login_screen.dart';

void main() {
  test('plain HTTP server addresses get a warning', () {
    expect(insecureServer('http://192.168.1.20:8080'), isTrue);
    expect(insecureServer('  HTTP://split.example.com'), isTrue);
    expect(insecureServer('https://split.example.com'), isFalse);
    expect(insecureServer(''), isFalse);
  });
}
