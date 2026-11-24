import 'package:splitspends/features/expenses/domain/expense.dart';

/// Route paths and typed arguments.
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

/// Arguments for the expense editor (`extra` of [Routes.newExpense]).
class ExpenseEditorArgs {
  const ExpenseEditorArgs({
    this.groupId,
    this.friendId,
    this.existing,
    this.draft,
  });

  final String? groupId;
  final String? friendId;

  /// Editing an existing expense.
  final Expense? existing;

  /// Prefill from AI (receipt scan or text draft).
  final ExpenseInput? draft;
}

/// Arguments for the settle-up screen.
class SettleArgs {
  const SettleArgs({
    this.groupId,
    this.fromUserId,
    this.toUserId,
    this.amount,
    this.currency,
    this.friendId,
  });

  final String? groupId;
  final String? fromUserId;
  final String? toUserId;
  final int? amount;
  final String? currency;
  final String? friendId;
}
