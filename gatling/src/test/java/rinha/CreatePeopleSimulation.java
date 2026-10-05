package rinha;

import io.gatling.javaapi.core.FeederBuilder;
import io.gatling.javaapi.core.ScenarioBuilder;
import io.gatling.javaapi.core.Simulation;
import io.gatling.javaapi.http.HttpProtocolBuilder;

import java.time.Instant;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

import static io.gatling.javaapi.core.CoreDsl.atOnceUsers;
import static io.gatling.javaapi.core.CoreDsl.feed;
import static io.gatling.javaapi.core.CoreDsl.global;
import static io.gatling.javaapi.core.CoreDsl.listFeeder;
import static io.gatling.javaapi.core.CoreDsl.scenario;
import static io.gatling.javaapi.core.CoreDsl.repeat;
import static io.gatling.javaapi.core.CoreDsl.StringBody;
import static io.gatling.javaapi.http.HttpDsl.http;
import static io.gatling.javaapi.http.HttpDsl.status;

public class CreatePeopleSimulation extends Simulation {
  private static final String BASE_URL = System.getProperty("baseUrl", "http://localhost:8080");
  private static final int TOTAL = Integer.getInteger("total", 50_000);
  private static final int USERS = Integer.getInteger("users", 25);
  private static final int REQUESTS_PER_USER = requestsPerUser();

  private static int requestsPerUser() {
    if (USERS <= 0 || TOTAL <= 0 || TOTAL % USERS != 0) {
      throw new IllegalArgumentException("total must be positive and divisible by users");
    }
    return TOTAL / USERS;
  }

  private final HttpProtocolBuilder httpProtocol = http
      .baseUrl(BASE_URL)
      .acceptHeader("application/json")
      .contentTypeHeader("application/json")
      .userAgentHeader("Gatling/Rinha");

  private final FeederBuilder<Object> people = listFeeder(buildPeople());

  private static List<Map<String, Object>> buildPeople() {
    String runId = Long.toString(Instant.now().toEpochMilli());
    List<Map<String, Object>> result = new ArrayList<>(TOTAL);
    for (int i = 0; i < TOTAL; i++) {
      Map<String, Object> person = new HashMap<>();
      person.put("apelido", "p" + runId + String.format("%05d", i));
      person.put("nome", "Pessoa Go " + String.format("%05d", i));
      person.put("nascimento", "1990-01-15");
      person.put("stack", "[\"Go\",\"PostgreSQL\"]");
      result.add(person);
    }
    return result;
  }

  private final ScenarioBuilder createPeople = scenario("Create people")
      .repeat(REQUESTS_PER_USER).on(
          feed(people)
              .exec(http("POST /pessoas")
                  .post("/pessoas")
                  .body(StringBody("{\"apelido\":\"#{apelido}\",\"nome\":\"#{nome}\",\"nascimento\":\"#{nascimento}\",\"stack\":#{stack}}"))
                  .check(status().is(201)))
      );

  {
    setUp(createPeople.injectOpen(atOnceUsers(USERS)).protocols(httpProtocol))
        .assertions(
            global().successfulRequests().percent().is(100.0),
            global().responseTime().max().lt(30_000)
        );
  }
}
