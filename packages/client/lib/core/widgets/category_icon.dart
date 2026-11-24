import 'package:material_ui/material_ui.dart';

/// Icons a custom category may use, by Material Symbols name: the icons of
/// the built-in categories plus a few general ones. Same list and order as
/// `category.Icons` on the server.
const categoryIcons = <String, IconData>{
  'restaurant': Icons.restaurant_rounded,
  'local_grocery_store': Icons.local_grocery_store_rounded,
  'local_bar': Icons.local_bar_rounded,
  'local_cafe': Icons.local_cafe_rounded,
  'home': Icons.home_rounded,
  'bolt': Icons.bolt_rounded,
  'wifi': Icons.wifi_rounded,
  'cleaning_services': Icons.cleaning_services_rounded,
  'chair': Icons.chair_rounded,
  'build': Icons.build_rounded,
  'pets': Icons.pets_rounded,
  'local_gas_station': Icons.local_gas_station_rounded,
  'local_parking': Icons.local_parking_rounded,
  'local_taxi': Icons.local_taxi_rounded,
  'directions_bus': Icons.directions_bus_rounded,
  'directions_car': Icons.directions_car_rounded,
  'flight': Icons.flight_rounded,
  'hotel': Icons.hotel_rounded,
  'attractions': Icons.attractions_rounded,
  'movie': Icons.movie_rounded,
  'sports_esports': Icons.sports_esports_rounded,
  'music_note': Icons.music_note_rounded,
  'sports_soccer': Icons.sports_soccer_rounded,
  'subscriptions': Icons.subscriptions_rounded,
  'checkroom': Icons.checkroom_rounded,
  'redeem': Icons.redeem_rounded,
  'medical_services': Icons.medical_services_rounded,
  'school': Icons.school_rounded,
  'child_care': Icons.child_care_rounded,
  'shield': Icons.shield_rounded,
  'account_balance': Icons.account_balance_rounded,
  'shopping_bag': Icons.shopping_bag_rounded,
  'receipt_long': Icons.receipt_long_rounded,
  'label': Icons.label_rounded,
  'star': Icons.star_rounded,
  'favorite': Icons.favorite_rounded,
  'work': Icons.work_rounded,
  'fitness_center': Icons.fitness_center_rounded,
  'park': Icons.park_rounded,
  'celebration': Icons.celebration_rounded,
  'cake': Icons.cake_rounded,
  'savings': Icons.savings_rounded,
  'devices': Icons.devices_rounded,
  'menu_book': Icons.menu_book_rounded,
  'palette': Icons.palette_rounded,
  'volunteer_activism': Icons.volunteer_activism_rounded,
  'local_laundry_service': Icons.local_laundry_service_rounded,
  'content_cut': Icons.content_cut_rounded,
};

/// Built-in display group names to the id prefix that sets their tint.
const _groupPrefixes = {
  'Food and drink': 'food',
  'Home': 'home',
  'Transportation': 'transport',
  'Travel': 'travel',
  'Entertainment': 'entertainment',
  'Life': 'life',
};

/// Icon and tint for an expense category id like `food.dining`. Custom
/// categories pass their [icon] name and display [group].
({IconData icon, Color color}) categoryVisual(
  String? id, {
  bool payment = false,
  String? icon,
  String? group,
}) {
  if (payment) {
    return (icon: Icons.payments_rounded, color: const Color(0xFF0B8A5F));
  }
  if (icon != null && (id ?? '').startsWith('custom.')) {
    final tint = categoryVisual('${_groupPrefixes[group] ?? 'other'}.custom');
    return (
      icon: categoryIcons[icon] ?? Icons.label_rounded,
      color: tint.color,
    );
  }
  final value = id ?? '';
  const exact = <String, IconData>{
    'food.dining': Icons.restaurant_rounded,
    'food.groceries': Icons.local_grocery_store_rounded,
    'food.drinks': Icons.local_bar_rounded,
    'food.coffee': Icons.local_cafe_rounded,
    'home.rent': Icons.house_rounded,
    'home.utilities': Icons.bolt_rounded,
    'home.internet': Icons.wifi_rounded,
    'home.supplies': Icons.cleaning_services_rounded,
    'home.furniture': Icons.chair_rounded,
    'home.maintenance': Icons.handyman_rounded,
    'home.pets': Icons.pets_rounded,
    'transport.fuel': Icons.local_gas_station_rounded,
    'transport.parking': Icons.local_parking_rounded,
    'transport.taxi': Icons.local_taxi_rounded,
    'transport.transit': Icons.train_rounded,
    'transport.car': Icons.directions_car_rounded,
    'travel.flights': Icons.flight_rounded,
    'travel.lodging': Icons.hotel_rounded,
    'travel.activities': Icons.attractions_rounded,
    'entertainment.movies': Icons.movie_rounded,
    'entertainment.games': Icons.sports_esports_rounded,
    'entertainment.music': Icons.music_note_rounded,
    'entertainment.sports': Icons.sports_soccer_rounded,
    'entertainment.subscriptions': Icons.subscriptions_rounded,
    'life.clothing': Icons.checkroom_rounded,
    'life.gifts': Icons.card_giftcard_rounded,
    'life.medical': Icons.medical_services_rounded,
    'life.education': Icons.school_rounded,
    'life.childcare': Icons.child_friendly_rounded,
    'life.insurance': Icons.shield_rounded,
    'life.taxes': Icons.account_balance_rounded,
    'shopping': Icons.shopping_bag_rounded,
    'payment': Icons.payments_rounded,
  };
  final prefix = value.split('.').first;
  const groups = <String, (IconData, Color)>{
    'food': (Icons.restaurant_rounded, Color(0xFFF4511E)),
    'transport': (Icons.commute_rounded, Color(0xFF1E88E5)),
    'home': (Icons.home_rounded, Color(0xFF6D4C41)),
    'entertainment': (Icons.celebration_rounded, Color(0xFF8E24AA)),
    'travel': (Icons.luggage_rounded, Color(0xFF00897B)),
    'shopping': (Icons.shopping_bag_rounded, Color(0xFFD81B60)),
    'payment': (Icons.payments_rounded, Color(0xFF0B8A5F)),
    'life': (Icons.spa_rounded, Color(0xFF5E35B1)),
  };
  final fallback =
      groups[prefix] ?? (Icons.receipt_long_rounded, const Color(0xFF546E7A));
  return (icon: exact[value] ?? fallback.$1, color: fallback.$2);
}

/// Rounded square with a category icon.
class CategoryIcon extends StatelessWidget {
  const CategoryIcon({
    super.key,
    required this.category,
    this.payment = false,
    this.size = 44,
    this.icon,
    this.group,
  });

  final String? category;
  final bool payment;
  final double size;

  /// Icon name and display group of a custom category.
  final String? icon;
  final String? group;

  @override
  Widget build(BuildContext context) {
    final visual = categoryVisual(
      category,
      payment: payment,
      icon: icon,
      group: group,
    );
    final dark = Theme.of(context).brightness == Brightness.dark;
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        color: visual.color.withValues(alpha: dark ? 0.25 : 0.14),
        borderRadius: BorderRadius.circular(size * 0.32),
      ),
      child: Icon(visual.icon, color: visual.color, size: size * 0.52),
    );
  }
}
