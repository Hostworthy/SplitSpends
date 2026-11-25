import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/core/models/user.dart';

/// Who may create an account on the server.
enum SignupMode {
  open('Open', 'Anyone with an email address can sign up.'),
  invite(
    'Invite',
    'The allowed list, people a member added by email, people with a '
        'group invite link, and people an administrator or manager approved.',
  ),
  approval(
    'Approval',
    'Anyone can ask to join; an administrator or manager approves them '
        'first. The allowed list joins without waiting.',
  ),
  closed(
    'Closed',
    'Nobody new can sign up, not even the allowed list. Existing accounts '
        'still sign in.',
  );

  const SignupMode(this.label, this.description);

  final String label;
  final String description;

  static SignupMode named(String? name) =>
      values.where((m) => m.name == name).firstOrNull ?? invite;
}

/// Sign-up mode plus the allowed list of emails and `@domains`.
class SignupSettings {
  const SignupSettings({required this.mode, this.allowed = const []});

  factory SignupSettings.fromJson(JsonMap json) => SignupSettings(
    mode: SignupMode.named(json['mode'] as String?),
    allowed: (json['allowed'] as List? ?? const []).cast<String>(),
  );

  final SignupMode mode;
  final List<String> allowed;

  JsonMap toJson() => {'mode': mode.name, 'allowed': allowed};
}

/// Someone who proved their email and waits to join (or was decided).
class SignupRequest {
  const SignupRequest({
    required this.id,
    required this.email,
    required this.name,
    required this.status,
    required this.requestedAt,
  });

  factory SignupRequest.fromJson(JsonMap json) => SignupRequest(
    id: json['id'] as String,
    email: json['email'] as String,
    name: (json['name'] as String?) ?? '',
    status: (json['status'] as String?) ?? 'pending',
    requestedAt: DateTime.tryParse('${json['requestedAt']}') ?? DateTime.now(),
  );

  final String id;
  final String email;
  final String name;
  final String status;
  final DateTime requestedAt;

  String get label => name.trim().isEmpty ? email : name;
}

/// An account as administrators see it.
class Account {
  const Account({required this.user, required this.role});

  factory Account.fromJson(JsonMap json) => Account(
    user: User.fromJson(json),
    role: (json['role'] as String?) ?? 'user',
  );

  final User user;
  final String role;

  bool get disabled => user.status == 'disabled';
}

/// Server administration. Every call is checked by the server; managers
/// may only list and decide requests.
abstract interface class AdminRepository {
  Future<SignupSettings> signup();
  Future<SignupSettings> saveSignup(SignupSettings settings);
  Future<List<SignupRequest>> requests({String status = 'pending'});
  Future<SignupRequest> approve(String id);
  Future<SignupRequest> decline(String id);
  Future<List<Account>> accounts({String status = '', String query = ''});
  Future<Account> setRole(String userId, String role);
  Future<Account> disable(String userId);
  Future<Account> enable(String userId);
}
