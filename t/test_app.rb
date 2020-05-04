require 'test/unit'
require 'capybara/dsl'

Capybara.current_driver = :selenium_chrome_headless
Capybara.app_host = 'http://localhost:8080'

class CapybaraTestCase < Test::Unit::TestCase
  include Capybara::DSL

  def test_double_post_returns_error
    visit '/_admin/'
    fill_in 'new-post-name', with: 'a test post!'
    click_button 'New Post'

    visit '/_admin/'
    fill_in 'new-post-name', with: 'a test post!'
    click_button 'New Post'

    assert_match(/error: post already exists/, page.html)
  end

  def teardown
    Capybara.reset_sessions!
    Capybara.use_default_driver
  end
end
