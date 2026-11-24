import 'package:sembast/sembast.dart';

import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/core/storage/local_db.dart';
import 'package:splitspends/core/storage/local_ledger.dart';
import 'package:splitspends/features/account/domain/account_repository.dart';

/// The on-device profile.
class LocalAccountRepository implements AccountRepository {
  LocalAccountRepository(this._ledger);

  final LocalLedger _ledger;

  @override
  Future<Me> me() => _ledger.me();

  @override
  Future<Me> update({
    String? name,
    String? defaultCurrency,
    String? timezone,
    String? avatarColor,
    NotificationPrefs? notifications,
  }) async {
    final db = await _ledger.db;
    final next = (await _ledger.me(db)).copyWith(
      name: name,
      defaultCurrency: defaultCurrency,
      timezone: timezone,
      avatarColor: avatarColor,
      notifications: notifications,
    );
    await Stores.meta.record('profile').put(db, {
      ...next.toJson(),
      'timezone': next.timezone,
      'notifications': next.notifications.toJson(),
    });
    return next;
  }
}

/// Your totals from the on-device sheet.
class LocalBalancesRepository implements BalancesRepository {
  LocalBalancesRepository(this._ledger);

  final LocalLedger _ledger;

  @override
  Future<BalanceSummary> mine() async {
    final t = (await _ledger.sheet()).totals();
    return BalanceSummary.fromJson({
      'balances': LocalLedger.balancesJson(t.net),
      'owed': LocalLedger.balancesJson(t.owed),
      'owing': LocalLedger.balancesJson(t.owing),
    });
  }
}
