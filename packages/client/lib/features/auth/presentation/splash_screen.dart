import 'package:material_ui/material_ui.dart';

import 'package:splitspends/app/shell.dart';

class SplashScreen extends StatelessWidget {
  const SplashScreen({super.key});

  @override
  Widget build(BuildContext context) =>
      const Scaffold(body: Center(child: AppLogo(size: 56)));
}
