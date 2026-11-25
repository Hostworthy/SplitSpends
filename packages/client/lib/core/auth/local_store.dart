import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:splitspends/core/log.dart';

/// Device persistence for the session (server URL, token) and preferences.
abstract class LocalStore {
  Future<String?> readServerUrl();
  Future<void> writeServerUrl(String url);
  Future<String?> readToken();
  Future<void> writeToken(String? token);
  Future<String?> readThemeMode();
  Future<void> writeThemeMode(String mode);
  Future<String?> readThemeColor();
  Future<void> writeThemeColor(String color);

  /// `local` or `connected`; null until the user picks on the welcome screen.
  Future<String?> readAppMode();
  Future<void> writeAppMode(String? mode);
}

/// Token in the platform keychain/keystore, the rest in shared preferences.
class DeviceLocalStore implements LocalStore {
  DeviceLocalStore({FlutterSecureStorage? secure})
    : _secure = secure ?? const FlutterSecureStorage();

  static const _serverKey = 'server_url';
  static const _tokenKey = 'session_token';
  static const _themeKey = 'theme_mode';
  static const _colorKey = 'theme_color';
  static const _modeKey = 'app_mode';

  final FlutterSecureStorage _secure;

  @override
  Future<String?> readServerUrl() async =>
      (await SharedPreferences.getInstance()).getString(_serverKey);

  @override
  Future<void> writeServerUrl(String url) async =>
      (await SharedPreferences.getInstance()).setString(_serverKey, url);

  @override
  Future<String?> readToken() async {
    try {
      return await _secure.read(key: _tokenKey);
    } catch (error, stack) {
      // An unreadable keystore means signing in again; the log keeps why.
      log('auth.token.unreadable', error: error, stack: stack);
      return null;
    }
  }

  @override
  Future<void> writeToken(String? token) async {
    if (token == null) {
      await _secure.delete(key: _tokenKey);
    } else {
      await _secure.write(key: _tokenKey, value: token);
    }
  }

  @override
  Future<String?> readThemeMode() async =>
      (await SharedPreferences.getInstance()).getString(_themeKey);

  @override
  Future<void> writeThemeMode(String mode) async =>
      (await SharedPreferences.getInstance()).setString(_themeKey, mode);

  @override
  Future<String?> readThemeColor() async =>
      (await SharedPreferences.getInstance()).getString(_colorKey);

  @override
  Future<void> writeThemeColor(String color) async =>
      (await SharedPreferences.getInstance()).setString(_colorKey, color);

  @override
  Future<String?> readAppMode() async =>
      (await SharedPreferences.getInstance()).getString(_modeKey);

  @override
  Future<void> writeAppMode(String? mode) async {
    final prefs = await SharedPreferences.getInstance();
    if (mode == null) {
      await prefs.remove(_modeKey);
    } else {
      await prefs.setString(_modeKey, mode);
    }
  }
}

/// In-memory store for tests.
class MemoryLocalStore implements LocalStore {
  MemoryLocalStore({
    this.serverUrl,
    this.token,
    this.themeMode,
    this.themeColor,
    this.appMode,
  });

  String? serverUrl;
  String? token;
  String? themeMode;
  String? themeColor;
  String? appMode;

  @override
  Future<String?> readServerUrl() async => serverUrl;

  @override
  Future<void> writeServerUrl(String url) async => serverUrl = url;

  @override
  Future<String?> readToken() async => token;

  @override
  Future<void> writeToken(String? token) async => this.token = token;

  @override
  Future<String?> readThemeMode() async => themeMode;

  @override
  Future<void> writeThemeMode(String mode) async => themeMode = mode;

  @override
  Future<String?> readThemeColor() async => themeColor;

  @override
  Future<void> writeThemeColor(String color) async => themeColor = color;

  @override
  Future<String?> readAppMode() async => appMode;

  @override
  Future<void> writeAppMode(String? mode) async => appMode = mode;
}
