import 'package:path_provider/path_provider.dart';
import 'package:sembast/sembast_io.dart';

import 'package:splitspends/core/storage/local_db.dart';

Future<Database> openAppDatabase(String name) async {
  final dir = await getApplicationDocumentsDirectory();
  return openVersioned(databaseFactoryIo, '${dir.path}/$name');
}
