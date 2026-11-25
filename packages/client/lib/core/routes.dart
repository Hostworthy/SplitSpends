/// Route paths. Typed arguments live with the feature whose screen takes
/// them.
abstract final class Routes {
  static const splash = '/splash';
  static const welcome = '/welcome';
  static const welcomeLocal = '/welcome/local';
  static const login = '/login';
  static const verify = '/login/verify';
  static const importData = '/login/import';
  static const home = '/';
  static const groups = '/groups';
  static const friends = '/friends';
  static const activity = '/activity';
  static const account = '/account';
  static const tokens = '/account/tokens';
  static const categories = '/account/categories';
  static const admin = '/account/admin';
  static const newExpense = '/expenses/new';
  static const settle = '/settle';
  static const scan = '/scan';
  static const joinPrefix = '/join';

  static String group(String id) => '/groups/$id';
  static String groupSettings(String id) => '/groups/$id/settings';
  static String friend(String id) => '/friends/$id';
  static String expense(String id) => '/expenses/$id';
  static String editExpense(String id) => '/expenses/$id/edit';

  /// An invite link inside the app; [server] is set for links from another
  /// origin (pasted on a phone).
  static String join(String token, {String? server}) => server == null
      ? '$joinPrefix/$token'
      : '$joinPrefix/$token?server=${Uri.encodeQueryComponent(server)}';
}
