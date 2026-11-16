# Native conversation system

The implementation follows the explicit brief: warm white (#FAF9F6), ink black (#101512), restrained pine green (#287C66). It is an Operate surface. The first viewport leads with conversations, people and the latest message; compact native bars and generous message spacing keep content primary. The visual world and conversation-first viewport are pinned by the parent task.

UIKit navigation stacks, a four-section bottom tab bar, system sheets, document/photo pickers and previews follow iOS conventions. The repeated signatures are quiet rounded avatars, clear message bubbles and persistent demo identity. Labels use system Dynamic Type, user size adjustment, semantic foreground colors and one green action tint. Light/dark/system appearance share the same roles.

The template iconfont catalog was checked and its ten bundled glyphs do not include conversation/contact/settings semantics. The iOS implementation therefore uses Apple's bundled SF Symbols through Jobs APIs; it downloads no remote imagery. The bundled Logo.svg reuses the Jobs-authored Web/Public/Logo.svg from this repository, SHA256 recorded in the test summary. It is the immediate avatar fallback. Authenticated avatars replace it asynchronously only for the current connection/account/epoch; missing metadata, denied access and cell reuse clear the prior photo first. There is no remote placeholder dependency. Source: https://developer.apple.com/sf-symbols/ .

Verify with real Simulator captures, enlarged text and the settings appearance controls; desktop web is independently designed elsewhere.
