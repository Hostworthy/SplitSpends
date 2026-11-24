import 'package:splitspends/core/api/api_client.dart';
import 'package:splitspends/core/models/balance.dart';
import 'package:splitspends/core/models/user.dart';
import 'package:splitspends/features/account/domain/account_repository.dart';
import 'package:splitspends/features/account/domain/api_token.dart';

/// HTTP implementation used in connected mode.
class RemoteAccountRepository implements AccountRepository {
  RemoteAccountRepository(this._api);

  final ApiClient _api;

  @override
  Future<Me> me() => _api.getData('/me', (d) => Me.fromJson(d as JsonMap));

  @override
  Future<Me> update({
    String? name,
    String? defaultCurrency,
    String? timezone,
    String? avatarColor,
    bool? gravatar,
    NotificationPrefs? notifications,
  }) => _api.patchData('/me', {
    'name': ?name,
    'defaultCurrency': ?defaultCurrency,
    'timezone': ?timezone,
    'avatarColor': ?avatarColor,
    'gravatar': ?gravatar,
    'notifications': ?notifications?.toJson(),
  }, (d) => Me.fromJson(d as JsonMap));
}

class RemoteBalancesRepository implements BalancesRepository {
  RemoteBalancesRepository(this._api);

  final ApiClient _api;

  @override
  Future<BalanceSummary> mine() => _api.getData(
    '/me/balances',
    (d) => BalanceSummary.fromJson(d as JsonMap),
  );
}

class RemoteTokensRepository implements TokensRepository {
  RemoteTokensRepository(this._api);

  final ApiClient _api;

  @override
  Future<List<ApiToken>> list() async =>
      (await _api.getList('/me/tokens', ApiToken.fromJson)).items;

  @override
  Future<ApiToken> create(String name) => _api.postData('/me/tokens', {
    'name': name,
  }, (d) => ApiToken.fromJson(d as JsonMap));

  @override
  Future<void> revoke(String id) => _api.delete('/me/tokens/$id');
}
