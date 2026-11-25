import 'package:sembast_web/sembast_web.dart';

import 'package:splitspends/core/storage/local_db.dart';

Future<Database> openAppDatabase(String name) =>
    openVersioned(databaseFactoryWeb, name);
