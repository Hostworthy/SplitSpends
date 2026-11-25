import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:splitspends/core/auth/session.dart';
import 'package:splitspends/core/widgets/states.dart';
import 'package:splitspends/features/admin/presentation/people_tab.dart';
import 'package:splitspends/features/admin/presentation/requests_tab.dart';
import 'package:splitspends/features/admin/presentation/signup_tab.dart';

/// Administration for administrators (requests, people, sign-up) and
/// approvals only for managers. The server checks every action again.
class AdminScreen extends ConsumerWidget {
  const AdminScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final me = ref.watch(meProvider);
    if (me == null || !me.canApprove) {
      return Scaffold(
        appBar: AppBar(title: const Text('Administration')),
        body: const EmptyState(
          icon: Icons.lock_outline_rounded,
          title: 'Not available',
          message: 'Only administrators and managers can open this page.',
        ),
      );
    }
    if (!me.isAdmin) {
      return Scaffold(
        appBar: AppBar(title: const Text('Approvals')),
        body: const RequestsTab(),
      );
    }
    return DefaultTabController(
      length: 3,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Administration'),
          bottom: const TabBar(
            tabs: [
              Tab(text: 'Requests'),
              Tab(text: 'People'),
              Tab(text: 'Sign-up'),
            ],
          ),
        ),
        body: const TabBarView(
          children: [RequestsTab(), PeopleTab(), SignupTab()],
        ),
      ),
    );
  }
}
