// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// The scanner's offline engine: ZXing-C++ behind one C function (decision
// B21, ADR-0051). Dart hands over one luminance frame and a buffer; nothing
// else crosses the boundary and no C++ exception escapes.

#include <cstdint>
#include <cstdlib>
#include <cstring>
#include <exception>
#include <string>

#include "BarcodeFormat.h"
#include "ReadBarcode.h"

#if defined(_WIN32)
#define PLUX_EXPORT extern "C" __declspec(dllexport)
#else
#define PLUX_EXPORT extern "C" __attribute__((visibility("default")))
#endif

namespace {

// The schema's BarcodeFormat enum in permanent ID order, 1 to 13; bit
// (id - 1) of a format mask selects the format.
constexpr ZXing::BarcodeFormat kFormats[] = {
    ZXing::BarcodeFormat::QRCode,     ZXing::BarcodeFormat::Aztec,
    ZXing::BarcodeFormat::DataMatrix, ZXing::BarcodeFormat::PDF417,
    ZXing::BarcodeFormat::Code39,     ZXing::BarcodeFormat::Code93,
    ZXing::BarcodeFormat::Code128,    ZXing::BarcodeFormat::Codabar,
    ZXing::BarcodeFormat::EAN8,       ZXing::BarcodeFormat::EAN13,
    ZXing::BarcodeFormat::ITF,        ZXing::BarcodeFormat::UPCA,
    ZXing::BarcodeFormat::UPCE,
};
constexpr int kFormatCount = sizeof(kFormats) / sizeof(kFormats[0]);

int formatId(ZXing::BarcodeFormat format) {
  for (int i = 0; i < kFormatCount; i++) {
    if (kFormats[i] == format) return i + 1;
  }
  return 0;
}

}  // namespace

// Reads the first code in a luminance frame.
//
//   y, w, h, stride  one byte per pixel, rows `stride` bytes apart
//   formats          bit (id - 1) per schema BarcodeFormat ID; 0 reads all
//   out, outLen      receives the schema BarcodeFormat ID as byte 0, then
//                    the text as UTF-8 and a closing NUL
//
// Returns the text length in bytes; -1 when no code was found, -2 when `out`
// is too small, -3 for invalid arguments or an internal failure.
PLUX_EXPORT int plux_zxing_decode(const uint8_t* y, int w, int h, int stride,
                                  int formats, char* out, int outLen) {
  if (y == nullptr || out == nullptr || w <= 0 || h <= 0 || stride < w ||
      outLen < 2) {
    return -3;
  }
  try {
    ZXing::BarcodeFormats wanted;
    if (formats == 0) {
      wanted = ZXing::BarcodeFormat::Any;
    } else {
      for (int i = 0; i < kFormatCount; i++) {
        if ((formats >> i) & 1) wanted = wanted | kFormats[i];
      }
    }
    ZXing::ReaderOptions options;
    options.setFormats(wanted);
    options.setTryHarder(true);
    options.setTryRotate(true);
    options.setTryInvert(true);
    options.setTryDownscale(true);
    options.setMaxNumberOfSymbols(1);

    ZXing::ImageView image(y, w, h, ZXing::ImageFormat::Lum, stride, 1);
    ZXing::Barcodes found = ZXing::ReadBarcodes(image, options);
    for (const ZXing::Barcode& code : found) {
      if (!code.isValid()) continue;
      int id = formatId(code.format());
      if (id == 0) continue;
      std::string text = code.text();
      if (static_cast<long>(text.size()) + 2 > outLen) return -2;
      out[0] = static_cast<char>(id);
      std::memcpy(out + 1, text.data(), text.size());
      out[1 + text.size()] = '\0';
      return static_cast<int>(text.size());
    }
    return -1;
  } catch (const std::exception&) {
    return -3;
  } catch (...) {
    return -3;
  }
}

// Allocation for Dart, so the package needs no allocator library.
PLUX_EXPORT void* plux_zxing_alloc(int bytes) {
  return bytes > 0 ? std::malloc(static_cast<size_t>(bytes)) : nullptr;
}

PLUX_EXPORT void plux_zxing_free(void* p) { std::free(p); }
