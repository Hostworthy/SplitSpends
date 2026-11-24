import 'package:splitspends/core/models/json.dart';
import 'package:splitspends/features/groups/domain/group.dart';

/// A group invite link. [token] and [url] are only known right after the
/// link is created; the server keeps just a hash.
class Invite {
  const Invite({
    required this.id,
    this.token,
    this.url,
    this.createdBy,
    this.expiresAt,
    this.createdAt,
  });

  factory Invite.fromJson(JsonMap json) => Invite(
    id: json['id'] as String,
    token: json['token'] as String?,
    url: json['url'] as String?,
    createdBy: json['createdBy'] as String?,
    expiresAt: json['expiresAt'] as String?,
    createdAt: json['createdAt'] as String?,
  );

  final String id;
  final String? token;
  final String? url;
  final String? createdBy;
  final String? expiresAt;
  final String? createdAt;
}

/// What anyone holding a link sees before joining.
class InvitePreview {
  const InvitePreview({
    required this.groupName,
    this.emoji = '',
    this.color,
    this.memberCount = 0,
    this.invitedBy = '',
  });

  factory InvitePreview.fromJson(JsonMap json) => InvitePreview(
    groupName: (json['groupName'] as String?) ?? '',
    emoji: (json['emoji'] as String?) ?? '',
    color: json['color'] as String?,
    memberCount: (json['memberCount'] as num?)?.toInt() ?? 0,
    invitedBy: (json['invitedBy'] as String?) ?? '',
  );

  final String groupName;
  final String emoji;
  final String? color;
  final int memberCount;
  final String invitedBy;
}

/// An invite link split into the server it belongs to and its token.
class JoinLink {
  const JoinLink({required this.token, this.server});

  /// Accepts a full link (`https://split.example.com/join/<token>`) or a bare
  /// token. Returns null for anything else.
  static JoinLink? parse(String input) {
    final text = input.trim();
    if (text.isEmpty) return null;
    if (_token.hasMatch(text)) return JoinLink(token: text);
    final uri = Uri.tryParse(text.contains('://') ? text : 'https://$text');
    if (uri == null || uri.host.isEmpty) return null;
    final segments = uri.pathSegments.where((s) => s.isNotEmpty).toList();
    final at = segments.lastIndexOf('join');
    if (at < 0 || at != segments.length - 2) return null;
    final token = segments.last;
    if (!_token.hasMatch(token)) return null;
    final prefix = segments.sublist(0, at).join('/');
    final origin = uri.origin + (prefix.isEmpty ? '' : '/$prefix');
    return JoinLink(token: token, server: origin);
  }

  static final _token = RegExp(r'^[A-Za-z0-9_-]{20,128}$');

  final String token;

  /// The server's address, null when only a token was given.
  final String? server;
}

/// Invite links need a server.
abstract interface class InvitesRepository {
  Future<Invite> create(String groupId);

  /// Active links, without tokens.
  Future<List<Invite>> list(String groupId);

  Future<void> revoke(String groupId, String inviteId);

  /// Joins the group behind [token] (nothing changes if already a member).
  Future<Group> accept(String token);
}

/// The public preview, read before signing in, so it takes the server
/// explicitly like login does.
abstract interface class InvitePreviews {
  Future<InvitePreview> preview(String serverUrl, String token);
}
