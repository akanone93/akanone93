// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The permanent IDs of the widgets this package builds and of their props,
/// events and slots. They are those of the descriptors in
/// `schema/widgets`; a test compares them.
library;

/// The widget IDs of the package's descriptors.
abstract final class PluxComponentIds {
  /// `Table`.
  static const int table = 111;

  /// `ReorderableListView`.
  static const int reorderableListView = 114;

  /// `NestedScrollView`.
  static const int nestedScrollView = 115;

  /// `DataTable`.
  static const int dataTable = 117;

  /// `Stepper`.
  static const int stepper = 118;

  /// `SearchBar`.
  static const int searchBar = 122;

  /// `SearchAnchor`.
  static const int searchAnchor = 123;

  /// `Autocomplete`.
  static const int autocomplete = 124;

  /// `CalendarDatePicker`.
  static const int calendarDatePicker = 125;

  /// `NavigationRail`.
  static const int navigationRail = 132;

  /// `Draggable`.
  static const int draggable = 134;

  /// `DragTarget`.
  static const int dragTarget = 135;

  /// `InteractiveViewer`.
  static const int interactiveViewer = 136;

  /// `CupertinoPicker`.
  static const int cupertinoPicker = 156;

  /// `CupertinoDatePicker`.
  static const int cupertinoDatePicker = 157;

  /// `CupertinoContextMenu`.
  static const int cupertinoContextMenu = 158;

  /// `AmountInput`.
  static const int amountInput = 159;

  /// `MoneyText`.
  static const int moneyText = 160;

  /// `MaskedText`.
  static const int maskedText = 161;

  /// `PhoneInput`.
  static const int phoneInput = 162;

  /// `IbanInput`.
  static const int ibanInput = 163;

  /// `CountryPicker`.
  static const int countryPicker = 164;

  /// `CurrencyPicker`.
  static const int currencyPicker = 165;

  /// `PriceTag`.
  static const int priceTag = 166;

  /// `CartSummary`.
  static const int cartSummary = 167;

  /// `TransactionListItem`.
  static const int transactionListItem = 168;

  /// `AccountCard`.
  static const int accountCard = 169;

  /// `PaymentCard`.
  static const int paymentCard = 170;

  /// `ConsentCheckbox`.
  static const int consentCheckbox = 171;

  /// `SearchField`.
  static const int searchField = 172;

  /// `FilterBar`.
  static const int filterBar = 173;

  /// `SortSheet`.
  static const int sortSheet = 174;

  /// `InfiniteList`.
  static const int infiniteList = 175;

  /// `WizardForm`.
  static const int wizardForm = 176;

  /// `OnboardingPager`.
  static const int onboardingPager = 177;

  /// `SettingsList`.
  static const int settingsList = 178;

  /// `ProfileHeader`.
  static const int profileHeader = 179;

  /// `StatusTimeline`.
  static const int statusTimeline = 180;

  /// `LiveEtaCard`.
  static const int liveEtaCard = 181;

  /// `ProductCard`.
  static const int productCard = 182;

  /// `QuantityStepper`.
  static const int quantityStepper = 183;

  /// `Carousel`.
  static const int carousel = 184;

  /// `PromoBanner`.
  static const int promoBanner = 185;

  /// `CountdownTimer`.
  static const int countdownTimer = 186;

  /// `RatingBar`.
  static const int ratingBar = 187;

  /// `RatingInput`.
  static const int ratingInput = 188;

  /// `ImageGallery`.
  static const int imageGallery = 189;

  /// `FileUploader`.
  static const int fileUploader = 190;
}

/// The IDs of one widget and its members, by name.
final class ComponentIds {
  /// Creates the IDs of one widget.
  const ComponentIds(
    this.id, {
    this.props = const {},
    this.events = const {},
    this.slots = const {},
  });

  /// The widget ID.
  final int id;

  /// Prop IDs by name.
  final Map<String, int> props;

  /// Event IDs by name.
  final Map<String, int> events;

  /// Slot IDs by name.
  final Map<String, int> slots;

  /// The ID of prop [name].
  int p(String name) => props[name]!;

  /// The ID of event [name].
  int e(String name) => events[name]!;

  /// The ID of slot [name].
  int s(String name) => slots[name]!;
}

/// Every widget of the package by type, for the test that compares the IDs
/// with the descriptors.
const Map<String, ComponentIds> componentIds = {
  'AccountCard': ComponentIds(
    PluxComponentIds.accountCard,
    props: {
      'name': 1,
      'maskedNumber': 2,
      'balance': 3,
      'flipped': 4,
      'backLabel': 5,
    },
    events: {'onFlip': 1},
  ),
  'AmountInput': ComponentIds(
    PluxComponentIds.amountInput,
    props: {
      'label': 1,
      'currency': 2,
      'value': 3,
      'min': 4,
      'max': 5,
      'rangeMessage': 6,
    },
    events: {'onChanged': 1},
  ),
  'Autocomplete': ComponentIds(
    PluxComponentIds.autocomplete,
    props: {
      'options': 1,
      'hintText': 2,
      'optionsMaxHeight': 3,
      'initialValue': 4,
    },
    events: {'onSelected': 1},
  ),
  'CalendarDatePicker': ComponentIds(
    PluxComponentIds.calendarDatePicker,
    props: {'initialDate': 1, 'firstDate': 2, 'lastDate': 3, 'currentDate': 4},
    events: {'onDateChanged': 1, 'onDisplayedMonthChanged': 2},
  ),
  'Carousel': ComponentIds(
    PluxComponentIds.carousel,
    props: {
      'autoplay': 1,
      'intervalMs': 2,
      'indicators': 3,
      'semanticLabel': 4,
    },
    events: {'onPageChanged': 1},
  ),
  'CartSummary': ComponentIds(
    PluxComponentIds.cartSummary,
    props: {
      'lines': 1,
      'subtotal': 2,
      'tax': 3,
      'total': 4,
      'subtotalLabel': 5,
      'taxLabel': 6,
      'totalLabel': 7,
      'checkoutLabel': 8,
    },
    events: {'onCheckout': 1},
    slots: {'checkout': 1},
  ),
  'ConsentCheckbox': ComponentIds(
    PluxComponentIds.consentCheckbox,
    props: {'label': 1, 'documentVersion': 2, 'linkLabel': 3, 'value': 4},
    events: {'onChanged': 1, 'onAccepted': 2, 'onLinkTap': 3},
  ),
  'CountdownTimer': ComponentIds(
    PluxComponentIds.countdownTimer,
    props: {'endsAt': 1, 'format': 2},
    events: {'onFinished': 1},
  ),
  'CountryPicker': ComponentIds(
    PluxComponentIds.countryPicker,
    props: {'value': 1, 'searchHint': 2},
    events: {'onSelected': 1},
  ),
  'CupertinoContextMenu': ComponentIds(
    PluxComponentIds.cupertinoContextMenu,
    props: {'actions': 1, 'enableHapticFeedback': 2},
    events: {'onAction': 1},
    slots: {'child': 1},
  ),
  'CupertinoDatePicker': ComponentIds(
    PluxComponentIds.cupertinoDatePicker,
    props: {
      'mode': 1,
      'initialDateTime': 2,
      'minimumDate': 3,
      'maximumDate': 4,
      'minimumYear': 5,
      'maximumYear': 6,
      'minuteInterval': 7,
      'use24hFormat': 8,
      'backgroundColor': 9,
      'showDayOfWeek': 10,
      'showTimeSeparator': 11,
      'itemExtent': 12,
    },
    events: {'onDateTimeChanged': 1},
  ),
  'CupertinoPicker': ComponentIds(
    PluxComponentIds.cupertinoPicker,
    props: {
      'itemExtent': 1,
      'selectedIndex': 2,
      'diameterRatio': 3,
      'backgroundColor': 4,
      'offAxisFraction': 5,
      'useMagnifier': 6,
      'magnification': 7,
      'squeeze': 8,
      'looping': 9,
    },
    events: {'onSelectedItemChanged': 1},
  ),
  'CurrencyPicker': ComponentIds(
    PluxComponentIds.currencyPicker,
    props: {'value': 1, 'currencies': 2, 'searchHint': 3},
    events: {'onSelected': 1},
  ),
  'DataTable': ComponentIds(
    PluxComponentIds.dataTable,
    props: {
      'columns': 1,
      'rows': 2,
      'selectedRows': 3,
      'sortColumnIndex': 4,
      'sortAscending': 5,
      'showCheckboxColumn': 6,
      'decoration': 7,
      'dataRowColor': 8,
      'dataRowMinHeight': 9,
      'dataRowMaxHeight': 10,
      'dataTextStyle': 11,
      'headingRowColor': 12,
      'headingRowHeight': 13,
      'headingTextStyle': 14,
      'horizontalMargin': 15,
      'columnSpacing': 16,
      'showBottomBorder': 17,
      'dividerThickness': 18,
      'checkboxHorizontalMargin': 19,
      'border': 20,
      'clipBehavior': 21,
    },
    events: {'onSort': 1, 'onSelectChanged': 2, 'onSelectAll': 3},
  ),
  'DragTarget': ComponentIds(
    PluxComponentIds.dragTarget,
    props: {'accepts': 1, 'highlightColor': 2, 'hitTestBehavior': 3},
    events: {'onAccept': 1, 'onLeave': 2},
    slots: {'child': 1},
  ),
  'Draggable': ComponentIds(
    PluxComponentIds.draggable,
    props: {
      'data': 1,
      'axis': 2,
      'feedbackOffset': 3,
      'maxSimultaneousDrags': 4,
      'ignoringFeedbackSemantics': 5,
      'ignoringFeedbackPointer': 6,
      'rootOverlay': 7,
      'hitTestBehavior': 8,
    },
    events: {
      'onDragStarted': 1,
      'onDragEnd': 2,
      'onDragCompleted': 3,
      'onDraggableCanceled': 4,
    },
    slots: {'child': 1, 'feedback': 2, 'childWhenDragging': 3},
  ),
  'FileUploader': ComponentIds(
    PluxComponentIds.fileUploader,
    props: {
      'dataSource': 1,
      'operation': 2,
      'accept': 3,
      'maxBytes': 4,
      'label': 5,
      'uploadLabel': 6,
    },
    events: {'onUploaded': 1, 'onFailed': 2},
  ),
  'FilterBar': ComponentIds(
    PluxComponentIds.filterBar,
    props: {'filters': 1},
    events: {'onToggle': 1},
  ),
  'IbanInput': ComponentIds(
    PluxComponentIds.ibanInput,
    props: {'label': 1, 'value': 2, 'invalidMessage': 3},
    events: {'onChanged': 1},
  ),
  'ImageGallery': ComponentIds(
    PluxComponentIds.imageGallery,
    props: {'images': 1, 'initialPage': 2, 'indicators': 3, 'semanticLabel': 4},
    events: {'onPageChanged': 1},
  ),
  'InfiniteList': ComponentIds(
    PluxComponentIds.infiniteList,
    props: {'items': 1, 'status': 2, 'hasMore': 3, 'padding': 4},
    events: {'onEndReached': 1},
    slots: {'item': 1, 'empty': 2, 'loading': 3, 'error': 4},
  ),
  'InteractiveViewer': ComponentIds(
    PluxComponentIds.interactiveViewer,
    props: {
      'clipBehavior': 1,
      'panAxis': 2,
      'boundaryMargin': 3,
      'constrained': 4,
      'maxScale': 5,
      'minScale': 6,
      'interactionEndFrictionCoefficient': 7,
      'panEnabled': 8,
      'scaleEnabled': 9,
      'scaleFactor': 10,
      'alignment': 11,
      'trackpadScrollCausesScale': 12,
    },
    events: {'onInteractionStart': 1, 'onInteractionEnd': 2},
    slots: {'child': 1},
  ),
  'LiveEtaCard': ComponentIds(
    PluxComponentIds.liveEtaCard,
    props: {
      'title': 1,
      'etaMinutes': 2,
      'status': 3,
      'minutesLabel': 4,
      'icon': 5,
    },
  ),
  'MaskedText': ComponentIds(
    PluxComponentIds.maskedText,
    props: {
      'value': 1,
      'mask': 2,
      'visibleDigits': 3,
      'revealable': 4,
      'revealLabel': 5,
    },
    events: {'onReveal': 1},
  ),
  'MoneyText': ComponentIds(
    PluxComponentIds.moneyText,
    props: {'value': 1, 'display': 2, 'signDisplay': 3, 'style': 4},
  ),
  'NavigationRail': ComponentIds(
    PluxComponentIds.navigationRail,
    props: {
      'destinations': 1,
      'selectedIndex': 2,
      'extended': 3,
      'labelType': 4,
      'backgroundColor': 5,
      'elevation': 6,
      'groupAlignment': 7,
      'minWidth': 8,
      'minExtendedWidth': 9,
      'useIndicator': 10,
      'indicatorColor': 11,
    },
    events: {'onDestinationSelected': 1},
    slots: {'leading': 1, 'trailing': 2},
  ),
  'NestedScrollView': ComponentIds(
    PluxComponentIds.nestedScrollView,
    props: {
      'scrollDirection': 1,
      'reverse': 2,
      'physics': 3,
      'dragStartBehavior': 4,
      'floatHeaderSlivers': 5,
      'clipBehavior': 6,
      'hitTestBehavior': 7,
    },
    slots: {'headerSlivers': 1, 'body': 2},
  ),
  'OnboardingPager': ComponentIds(
    PluxComponentIds.onboardingPager,
    props: {'currentPage': 1, 'nextLabel': 2, 'doneLabel': 3},
    events: {'onPageChanged': 1, 'onDone': 2},
  ),
  'PaymentCard': ComponentIds(
    PluxComponentIds.paymentCard,
    props: {
      'brand': 1,
      'maskedNumber': 2,
      'expiry': 3,
      'holder': 4,
      'flipped': 5,
      'backLabel': 6,
    },
    events: {'onFlip': 1},
  ),
  'PhoneInput': ComponentIds(
    PluxComponentIds.phoneInput,
    props: {'label': 1, 'value': 2, 'region': 3, 'invalidMessage': 4},
    events: {'onChanged': 1},
  ),
  'PriceTag': ComponentIds(
    PluxComponentIds.priceTag,
    props: {'price': 1, 'originalPrice': 2, 'wasLabel': 3},
  ),
  'ProductCard': ComponentIds(
    PluxComponentIds.productCard,
    props: {
      'image': 1,
      'title': 2,
      'price': 3,
      'originalPrice': 4,
      'badge': 5,
      'addToCartLabel': 6,
    },
    events: {'onTap': 1, 'onAddToCart': 2},
  ),
  'ProfileHeader': ComponentIds(
    PluxComponentIds.profileHeader,
    props: {'name': 1, 'subtitle': 2, 'avatar': 3, 'radius': 4},
    slots: {'actions': 1},
  ),
  'PromoBanner': ComponentIds(
    PluxComponentIds.promoBanner,
    props: {'title': 1, 'subtitle': 2, 'image': 3, 'ctaLabel': 4, 'endsAt': 5},
    events: {'onTap': 1},
  ),
  'QuantityStepper': ComponentIds(
    PluxComponentIds.quantityStepper,
    props: {
      'value': 1,
      'min': 2,
      'max': 3,
      'step': 4,
      'decreaseLabel': 5,
      'increaseLabel': 6,
    },
    events: {'onChanged': 1},
  ),
  'RatingBar': ComponentIds(
    PluxComponentIds.ratingBar,
    props: {'value': 1, 'max': 2, 'ofLabel': 3, 'semanticLabel': 4},
  ),
  'RatingInput': ComponentIds(
    PluxComponentIds.ratingInput,
    props: {'value': 1, 'max': 2, 'starLabel': 3},
    events: {'onChanged': 1},
  ),
  'ReorderableListView': ComponentIds(
    PluxComponentIds.reorderableListView,
    props: {
      'items': 1,
      'scrollDirection': 2,
      'reverse': 3,
      'physics': 4,
      'shrinkWrap': 5,
      'padding': 6,
      'buildDefaultDragHandles': 7,
      'itemExtent': 8,
      'anchor': 9,
      'dragStartBehavior': 11,
      'keyboardDismissBehavior': 12,
      'clipBehavior': 13,
      'autoScrollerVelocityScalar': 14,
      'status': 15,
    },
    events: {'onReorder': 1, 'onReorderStart': 2, 'onReorderEnd': 3},
    slots: {
      'item': 1,
      'header': 2,
      'footer': 3,
      'empty': 4,
      'loading': 5,
      'error': 6,
    },
  ),
  'SearchAnchor': ComponentIds(
    PluxComponentIds.searchAnchor,
    props: {
      'suggestions': 1,
      'isFullScreen': 2,
      'viewHintText': 3,
      'hintText': 4,
    },
    events: {'onSelected': 1},
  ),
  'SearchBar': ComponentIds(
    PluxComponentIds.searchBar,
    props: {'value': 1, 'hintText': 2},
    events: {'onChanged': 1, 'onSubmitted': 2, 'onTap': 3},
    slots: {'leading': 1, 'trailing': 2},
  ),
  'SearchField': ComponentIds(
    PluxComponentIds.searchField,
    props: {'value': 1, 'hint': 2, 'debounceMs': 3, 'suggestions': 4},
    events: {'onSearch': 1, 'onSuggestionSelected': 2},
  ),
  'SettingsList': ComponentIds(
    PluxComponentIds.settingsList,
    props: {'sections': 1},
  ),
  'SortSheet': ComponentIds(
    PluxComponentIds.sortSheet,
    props: {'title': 1, 'options': 2, 'selected': 3},
    events: {'onSelected': 1},
  ),
  'StatusTimeline': ComponentIds(
    PluxComponentIds.statusTimeline,
    props: {'steps': 1},
  ),
  'Stepper': ComponentIds(
    PluxComponentIds.stepper,
    props: {
      'steps': 1,
      'type': 2,
      'currentStep': 3,
      'physics': 4,
      'elevation': 5,
      'margin': 6,
      'connectorColor': 7,
      'connectorThickness': 8,
      'stepIconHeight': 9,
      'stepIconWidth': 10,
      'stepIconMargin': 11,
      'clipBehavior': 12,
      'headerPadding': 13,
      'contentPadding': 14,
    },
    events: {'onStepTapped': 1, 'onStepContinue': 2, 'onStepCancel': 3},
  ),
  'Table': ComponentIds(
    PluxComponentIds.table,
    props: {
      'columns': 1,
      'columnWidths': 2,
      'defaultColumnWidth': 3,
      'textDirection': 4,
      'border': 5,
      'defaultVerticalAlignment': 6,
      'textBaseline': 7,
    },
  ),
  'TransactionListItem': ComponentIds(
    PluxComponentIds.transactionListItem,
    props: {
      'title': 1,
      'subtitle': 2,
      'amount': 3,
      'status': 4,
      'date': 5,
      'statusLabel': 6,
    },
    events: {'onTap': 1},
  ),
  'WizardForm': ComponentIds(
    PluxComponentIds.wizardForm,
    props: {
      'titles': 1,
      'currentStep': 2,
      'stepFields': 3,
      'backLabel': 4,
      'nextLabel': 5,
      'submitLabel': 6,
    },
    events: {'onNext': 1, 'onBack': 2, 'onSubmit': 3},
  ),
};
