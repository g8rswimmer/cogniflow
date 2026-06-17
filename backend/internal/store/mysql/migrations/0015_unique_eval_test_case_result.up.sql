ALTER TABLE eval_test_case_results
    ADD UNIQUE KEY uq_etcr_run_case (eval_run_id, test_case_id);
