import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/account/domain/api_token.dart';

/// Your profile and preferences.
abstract interface class AccountRepository {
  Future<Me> me();

  Future<Me> update({
    String? name,
    String? defaultCurrency,
    String? timezone,
    String? avatarColor,
    bool? gravatar,
    NotificationPrefs? notifications,
  });
}

/// Your totals across every group and friend.
abstract interface class BalancesRepository {
  Future<BalanceSummary> mine();
}

/// Personal API tokens (connected mode only: they authenticate MCP clients).
abstract interface class TokensRepository {
  Future<List<ApiToken>> list();

  /// The returned token carries the secret once.
  Future<ApiToken> create(String name);

  Future<void> revoke(String id);
}
