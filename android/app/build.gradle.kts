import java.util.Properties
import java.security.KeyStore
import java.security.MessageDigest

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.plugin.compose")
    id("org.jetbrains.kotlin.plugin.serialization")
}
val tapDeckVersions = Properties().apply {
    rootProject.file("../version.properties").inputStream().use { load(it) }
}
val tapDeckAndroidVersion = requireNotNull(tapDeckVersions.getProperty("controller.android.version"))
val tapDeckAndroidCode = requireNotNull(tapDeckVersions.getProperty("controller.android.versionCode")).toInt()
require(tapDeckAndroidVersion.matches(Regex("(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)")))
require(tapDeckAndroidCode in 1..2100000000)
val releaseSigningNames = listOf("STORE_FILE", "STORE_PASSWORD", "KEY_ALIAS", "KEY_PASSWORD")
val releaseSigningValues = releaseSigningNames.associateWith {
    providers.environmentVariable("TAPDECK_ANDROID_$it").orNull
}
val releaseSigningComplete = releaseSigningValues.values.all { !it.isNullOrEmpty() }
val verifyReleaseSigning = tasks.register("verifyReleaseSigning") {
    doLast {
        check(releaseSigningComplete) {
            "Release signing is required. Configure TAPDECK_ANDROID_* or use scripts/build-android.ps1."
        }
        val store = file(requireNotNull(releaseSigningValues["STORE_FILE"]))
        check(store.isFile) { "Release keystore not found" }
        val keys = KeyStore.getInstance(store, requireNotNull(releaseSigningValues["STORE_PASSWORD"]).toCharArray())
        val alias = requireNotNull(releaseSigningValues["KEY_ALIAS"])
        check(keys.isKeyEntry(alias) && keys.getKey(alias, requireNotNull(releaseSigningValues["KEY_PASSWORD"]).toCharArray()) != null) {
            "Release signing key not found"
        }
        val certificate = requireNotNull(keys.getCertificate(alias)) { "Release certificate not found" }
        val fingerprint = MessageDigest.getInstance("SHA-256").digest(certificate.encoded)
            .joinToString("") { "%02x".format(it.toInt() and 255) }
        val expected = rootProject.file("release-certificate.sha256").readText().trim()
        check(expected.matches(Regex("[0-9a-f]{64}")) && fingerprint == expected) {
            "Release certificate differs from android/release-certificate.sha256"
        }
    }
}
android {
    namespace = "com.yuncii.tapdeck"
    compileSdk = 37
    defaultConfig {
        applicationId = "com.yuncii.tapdeck"
        minSdk = 26
        targetSdk = 37
        versionCode = tapDeckAndroidCode
        versionName = tapDeckAndroidVersion
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }
    testBuildType = "release"
    signingConfigs {
        if (releaseSigningComplete) create("tapdeckRelease") {
            storeFile = file(requireNotNull(releaseSigningValues["STORE_FILE"]))
            storePassword = releaseSigningValues["STORE_PASSWORD"]
            keyAlias = releaseSigningValues["KEY_ALIAS"]
            keyPassword = releaseSigningValues["KEY_PASSWORD"]
        }
    }
    buildFeatures { compose = true; buildConfig = true }
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17; targetCompatibility = JavaVersion.VERSION_17 }
    buildTypes {
        release {
            isDebuggable = false
            isJniDebuggable = false
            isProfileable = false
            isMinifyEnabled = false
            if (releaseSigningComplete) signingConfig = signingConfigs.getByName("tapdeckRelease")
        }
    }
}
tasks.matching { it.name == "preReleaseBuild" }.configureEach { dependsOn(verifyReleaseSigning) }
kotlin { compilerOptions { jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17) } }
dependencies {
    implementation(platform("androidx.compose:compose-bom:2025.04.01"))
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.activity:activity-compose:1.10.1")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.9.0")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.9.0")
    implementation("androidx.datastore:datastore-preferences:1.1.7")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.10.2")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.9.0")
    implementation("com.squareup.okhttp3:okhttp:5.5.0")
    implementation("com.journeyapps:zxing-android-embedded:4.3.0")
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.10.2")
    androidTestImplementation("androidx.test:runner:1.6.2")
    androidTestImplementation("androidx.test:core:1.6.1")
    androidTestImplementation("androidx.test.ext:junit:1.2.1")
}
