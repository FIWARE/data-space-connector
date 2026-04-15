package org.fiware.dataspace.it.components;

import org.junit.platform.suite.api.ConfigurationParameter;
import org.junit.platform.suite.api.IncludeEngines;
import org.junit.platform.suite.api.SelectClasspathResource;
import org.junit.platform.suite.api.Suite;

import static io.cucumber.junit.platform.engine.Constants.PLUGIN_PROPERTY_NAME;

/**
 * JUnit5 Suite entry point for Cucumber integration tests.
 *
 * <p>Cucumber tag filtering is controlled via the {@code cucumber.filter.tags} system property,
 * which can be set by Maven profiles (e.g., {@code -Plocal-test}, {@code -Pcentral-test},
 * {@code -Pdsp-test}). When no filter is set, all tests are executed.</p>
 *
 * <p>Available tags:</p>
 * <ul>
 *   <li>{@code @local} — Tests for the basic local deployment ({@code mvn clean deploy -Plocal})</li>
 *   <li>{@code @central} — Tests for the central marketplace deployment ({@code mvn clean deploy -Plocal,central})</li>
 *   <li>{@code @dsp} — Tests for the Dataspace Protocol deployment ({@code mvn clean deploy -Plocal,dsp})</li>
 * </ul>
 *
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
@Suite
@IncludeEngines("cucumber")
@SelectClasspathResource("it")
@ConfigurationParameter(key = PLUGIN_PROPERTY_NAME, value = "pretty")
public class RunCucumberTest {
}
