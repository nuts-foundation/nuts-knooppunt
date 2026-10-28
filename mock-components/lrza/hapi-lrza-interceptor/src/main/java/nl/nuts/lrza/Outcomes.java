package nl.nuts.lrza;

import org.hl7.fhir.r4.model.OperationOutcome;

/** OperationOutcomes for the mock's errors. */
final class Outcomes {

    private Outcomes() {
    }

    static OperationOutcome error(final String code, final String expression, final String diagnostics) {
        final OperationOutcome outcome = new OperationOutcome();
        add(outcome, code, expression, diagnostics);
        return outcome;
    }

    static void add(final OperationOutcome outcome, final String code, final String expression, final String diagnostics) {
        final OperationOutcome.OperationOutcomeIssueComponent issue = outcome.addIssue()
                .setSeverity(OperationOutcome.IssueSeverity.ERROR)
                .setCode(OperationOutcome.IssueType.fromCode(code))
                .setDiagnostics(diagnostics);
        if (expression != null) {
            issue.addExpression(expression);
        }
    }
}
