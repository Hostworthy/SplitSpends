import 'package:material_ui/material_ui.dart';

/// Icon and tint for an expense category id like `food.dining`.
({IconData icon, Color color}) categoryVisual(
  String? id, {
  bool payment = false,
}) {
  if (payment) {
    return (icon: Icons.payments_rounded, color: const Color(0xFF0B8A5F));
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
  final group = value.split('.').first;
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
      groups[group] ?? (Icons.receipt_long_rounded, const Color(0xFF546E7A));
  return (icon: exact[value] ?? fallback.$1, color: fallback.$2);
}

/// Rounded square with a category icon.
class CategoryIcon extends StatelessWidget {
  const CategoryIcon({
    super.key,
    required this.category,
    this.payment = false,
    this.size = 44,
  });

  final String? category;
  final bool payment;
  final double size;

  @override
  Widget build(BuildContext context) {
    final visual = categoryVisual(category, payment: payment);
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
