# Loomspan repository guidance

## Public API and compatibility

Loomspan's supported application-facing Java API is deliberately small and closed. It consists only of the public top-level types in `com.lokiscale.loomspan.api` that are allowlisted by `LoomspanPublicSurfaceArchitectureTest`.

- Treat changes to those allowlisted API types as compatibility-sensitive. Before changing or removing one, consider source, binary, and behavioral compatibility and add a compatibility shim when the project requires one.
- A Java `public` modifier does not by itself make a type supported API.
- Everything below `com.lokiscale.loomspan.internal`, including `public` classes, interfaces, records, constructors, and methods, is implementation detail. It may change or disappear without a compatibility shim. Do not preserve an internal type solely for compatibility, and do not recommend internal types to library consumers.
- Types in `com.lokiscale.loomspan.autoconfigure` are Spring Boot integration machinery and configuration binding types, not an application extension API. Their Java signatures do not require compatibility shims merely because Spring requires them to be public. User-visible configuration keys and documented configuration behavior are separate compatibility concerns.
- Loomspan currently exposes no supported Java SPI and no supported internal bean-replacement surface. Do not create an SPI, bean override contract, or additional public API accidentally.
- New application-facing API must live in `com.lokiscale.loomspan.api`, be deliberately added to the closed allowlist, be documented in the README, and have supported-surface tests. Prefer keeping a type internal unless consumers genuinely need it.
- Public API signatures must not expose types from `internal` or `autoconfigure` packages.

Run `LoomspanPublicSurfaceArchitectureTest` after changing production types. Its allowlists are the executable authority for the Java API classification; the README is the consumer-facing summary.
